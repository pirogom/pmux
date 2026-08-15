package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestSafeWriteWS_ConcurrentWriters(t *testing.T) {
	// Create test HTTP server that upgrades to WebSocket
	var serverConn *websocket.Conn
	var serverConnMu sync.Mutex
	ready := make(chan struct{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			t.Errorf("Accept failed: %v", err)
			return
		}
		c.SetReadLimit(16 * 1024 * 1024)
		serverConnMu.Lock()
		serverConn = c
		serverConnMu.Unlock()
		close(ready)

		// Read loop to drain incoming or wait until closed
		for {
			if _, _, err := c.Read(r.Context()); err != nil {
				break
			}
		}
	}))
	defer ts.Close()

	// Connect client
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, "ws"+ts.URL[4:], nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close(websocket.StatusNormalClosure, "")

	<-ready

	serverConnMu.Lock()
	sc := serverConn
	serverConnMu.Unlock()

	if sc == nil {
		t.Fatal("server conn is nil")
	}
	defer removeWSConn(sc)

	// Spawn multiple concurrent goroutines writing through safeWriteWS
	const numWriters = 10
	const writesPerWriter = 20
	var wg sync.WaitGroup

	// Client reader goroutine to count messages received
	receivedCount := 0
	var readMu sync.Mutex
	doneReading := make(chan struct{})
	go func() {
		defer close(doneReading)
		for {
			_, _, err := clientConn.Read(context.Background())
			if err != nil {
				break
			}
			readMu.Lock()
			receivedCount++
			total := receivedCount
			readMu.Unlock()
			if total == numWriters*writesPerWriter {
				break
			}
		}
	}()

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				payload := []byte("hello from concurrent writer")
				if err := safeWriteWS(sc, payload); err != nil {
					t.Errorf("writer %d write %d failed: %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()

	select {
	case <-doneReading:
		// success
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for client to receive all messages, got %d / %d", receivedCount, numWriters*writesPerWriter)
	}
}

func TestDecoupledWritePump_HighThroughputBurst(t *testing.T) {
	sendCh := make(chan []byte, 512)
	ready := make(chan struct{})
	var serverConn *websocket.Conn
	var serverConnMu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			t.Errorf("Accept failed: %v", err)
			return
		}
		c.SetReadLimit(16 * 1024 * 1024)
		serverConnMu.Lock()
		serverConn = c
		serverConnMu.Unlock()
		close(ready)

		// Start write pump
		go func() {
			for {
				select {
				case <-r.Context().Done():
					return
				case chunk, ok := <-sendCh:
					if !ok {
						return
					}
					var buf bytes.Buffer
					buf.Write(chunk)
				drainLoop:
					for buf.Len() < 64*1024 {
						select {
						case extra, ok := <-sendCh:
							if !ok {
								break drainLoop
							}
							buf.Write(extra)
						default:
							break drainLoop
						}
					}
					if err := safeWriteWS(c, buf.Bytes()); err != nil {
						return
					}
				}
			}
		}()

		// Drain incoming
		for {
			if _, _, err := c.Read(r.Context()); err != nil {
				break
			}
		}
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, "ws"+ts.URL[4:], nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	clientConn.SetReadLimit(16 * 1024 * 1024)
	defer clientConn.Close(websocket.StatusNormalClosure, "")

	<-ready

	serverConnMu.Lock()
	sc := serverConn
	serverConnMu.Unlock()
	defer removeWSConn(sc)

	// Pump 1000 chunks (simulating 1000 burst tokens of 1KB each = 1MB)
	const totalChunks = 1000
	const chunkSize = 1024
	testChunk := make([]byte, chunkSize)
	for i := range testChunk {
		testChunk[i] = 'A'
	}

	totalBytesReceived := 0
	var byteMu sync.Mutex
	doneReading := make(chan struct{})

	go func() {
		defer close(doneReading)
		for {
			_, msg, err := clientConn.Read(context.Background())
			if err != nil {
				break
			}
			byteMu.Lock()
			totalBytesReceived += len(msg)
			current := totalBytesReceived
			byteMu.Unlock()
			if current >= totalChunks*chunkSize {
				break
			}
		}
	}()

	for i := 0; i < totalChunks; i++ {
		sendCh <- testChunk
	}

	select {
	case <-doneReading:
		// Succeeded in receiving all data
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for burst data, received %d / %d bytes", totalBytesReceived, totalChunks*chunkSize)
	}
}
