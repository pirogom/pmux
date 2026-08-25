package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// newManualPane registers a pane without a real ConPTY so the canonical-size
// policy can be tested in isolation.
func newManualPane(sm *SessionManager, sessionID, paneID string, cols, rows int) *Pane {
	pane := newPane(paneID, sessionID, "", "cmd.exe", nil, cols, rows, nil, nil)
	if sess, ok := sm.sessions[sessionID]; ok {
		sess.Panes[paneID] = pane
	} else {
		sm.sessions[sessionID] = &Session{ID: sessionID, Name: sessionID, Panes: map[string]*Pane{paneID: pane}}
	}
	sm.panes[paneID] = pane
	return pane
}

// dialPaneWS connects a client to the pane's websocket endpoint and reads the
// mandatory initial pane-size frame (sent before preamble/history).
func dialPaneWS(t *testing.T, sm *SessionManager, paneID string) *websocket.Conn {
	t.Helper()
	s := &Server{sessionMgr: sm}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/pane/", s.handleWSPane)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws/pane/"+paneID, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
	return conn
}

func sendWSResize(t *testing.T, ctx context.Context, conn *websocket.Conn, cols, rows int) {
	t.Helper()
	msg := fmt.Sprintf(`{"type":"resize","cols":%d,"rows":%d}`, cols, rows)
	if err := conn.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
		t.Fatalf("write resize failed: %v", err)
	}
}

// readPaneSize reads frames until a pane-size control message arrives
// (binary terminal output produced by ConPTY resizes is drained), asserting
// the frame type and returning its dimensions.
func readPaneSize(t *testing.T, ctx context.Context, conn *websocket.Conn) (int, int) {
	t.Helper()
	for {
		mt, msg, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read pane-size failed: %v", err)
		}
		if mt == websocket.MessageBinary {
			continue // drain live terminal output; control frames are text
		}
		if mt != websocket.MessageText {
			t.Fatalf("expected text pane-size frame, got message type %v", mt)
		}
		var m struct {
			Type string `json:"type"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
		}
		if err := json.Unmarshal(msg, &m); err != nil {
			t.Fatalf("unmarshal pane-size failed: %v (%s)", err, msg)
		}
		if m.Type != "pane-size" {
			t.Fatalf("unexpected frame type %q: %s", m.Type, msg)
		}
		return m.Cols, m.Rows
	}
}

// expectNoPaneSize asserts that no text control frame arrives within the
// given window. Binary terminal output is drained and ignored. NOTE: reads
// use a plain background context because coder/websocket closes the
// connection when a read context expires.
func expectNoPaneSize(t *testing.T, ctx context.Context, conn *websocket.Conn, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		type readResult struct {
			mt  websocket.MessageType
			err error
		}
		readDone := make(chan readResult, 1)
		go func() {
			mt, _, err := conn.Read(context.Background())
			readDone <- readResult{mt, err}
		}()
		var timeout <-chan time.Time
		if remaining := deadline.Sub(time.Now()); remaining > 0 {
			timeout = time.After(remaining)
		} else {
			return
		}
		select {
		case res := <-readDone:
			if res.err != nil {
				return
			}
			if res.mt == websocket.MessageText {
				t.Fatalf("unexpected pane-size control frame")
			}
			// binary terminal data: drain and keep waiting
		case <-timeout:
			return
		}
	}
}

func waitForPaneSize(t *testing.T, sm *SessionManager, paneID string, wantCols, wantRows int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pane, ok := sm.GetPane(paneID)
		if !ok {
			t.Fatalf("pane %s not found", paneID)
		}
		pane.mu.Lock()
		c, r := pane.Cols, pane.Rows
		pane.mu.Unlock()
		if c == wantCols && r == wantRows {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pane did not reach %dx%d", wantCols, wantRows)
}

func TestCanonicalSize_LargestClientPolicy(t *testing.T) {
	sm := NewSessionManager()
	pane := newManualPane(sm, "s1", "p1", 80, 24)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// No clients attached -> canonical stays at the pane's current size
	if cols, rows := sm.HandleClientResize("p1", nil, 100, 40); cols != 80 || rows != 24 {
		t.Fatalf("unregistered client resize must not change canonical size, got %dx%d", cols, rows)
	}

	c1 := dialPaneWS(t, sm, "p1")
	c2 := dialPaneWS(t, sm, "p1")

	if cols, rows := readPaneSize(t, ctx, c1); cols != 80 || rows != 24 {
		t.Fatalf("initial pane-size for c1 must be 80x24, got %dx%d", cols, rows)
	}
	if cols, rows := readPaneSize(t, ctx, c2); cols != 80 || rows != 24 {
		t.Fatalf("initial pane-size for c2 must be 80x24, got %dx%d", cols, rows)
	}

	// c1 grows the canonical size up to its viewport -> broadcast to all
	sendWSResize(t, ctx, c1, 100, 40)
	if cols, rows := readPaneSize(t, ctx, c1); cols != 100 || rows != 40 {
		t.Fatalf("expected broadcast to grow to 100x40, got %dx%d", cols, rows)
	}

	// A smaller client resizing must NOT shrink the pane (no PTY flip-flop):
	// drain c2's pending broadcast first, then assert silence on resize.
	if cols, rows := readPaneSize(t, ctx, c2); cols != 100 || rows != 40 {
		t.Fatalf("c2 must receive the 100x40 broadcast, got %dx%d", cols, rows)
	}
	sendWSResize(t, ctx, c2, 60, 20)
	expectNoPaneSize(t, ctx, c2, 300*time.Millisecond)

	// c2 outgrows c1 -> canonical follows the new largest (asserted via the
	// session manager because expectNoPaneSize leaves a background reader)
	sendWSResize(t, ctx, c2, 120, 50)
	waitForPaneSize(t, sm, "p1", 120, 50)

	// Largest client disconnects -> canonical shrinks to the next largest
	_ = c2.Close(websocket.StatusNormalClosure, "")
	waitForPaneSize(t, sm, "p1", 100, 40)

	// Last client disconnects -> canonical keeps the last known size
	_ = c1.Close(websocket.StatusNormalClosure, "")
	waitForPaneSize(t, sm, "p1", 100, 40)
	_ = pane
}

func TestCanonicalSize_InvalidResizeIgnored(t *testing.T) {
	sm := NewSessionManager()
	pane := newManualPane(sm, "s1", "p1", 80, 24)
	c1 := dialPaneWS(t, sm, "p1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = readPaneSize(t, ctx, c1)

	cols, rows := sm.HandleClientResize("p1", nil, 5, 2)
	if cols != 0 || rows != 0 {
		t.Fatalf("tiny invalid resize should be ignored, got %dx%d", cols, rows)
	}

	cols, rows = sm.HandleClientResize("p1", nil, 0, 0)
	if cols != 0 || rows != 0 {
		t.Fatalf("zero resize should be ignored, got %dx%d", cols, rows)
	}

	pane.mu.Lock()
	defer pane.mu.Unlock()
	if pane.Cols != 80 || pane.Rows != 24 {
		t.Fatalf("pane size must remain 80x24, got %dx%d", pane.Cols, pane.Rows)
	}
}

func TestWSPane_SendsPaneSizeBeforeHistoryAndBroadcastOnChange(t *testing.T) {
	sm := NewSessionManager()
	sess, pane, err := sm.CreateSession("prof_ws_test", "WSPaneSizeSession", "cmd.exe", nil, "", 80, 24)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	defer sm.CloseSession(sess.ID)

	conn := dialPaneWS(t, sm, pane.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First frame must be the canonical pane-size control message (text),
	// preceding any binary history/preamble data
	if cols, rows := readPaneSize(t, ctx, conn); cols != 80 || rows != 24 {
		t.Fatalf("initial pane-size must be 80x24, got %dx%d", cols, rows)
	}

	// Growing resize -> broadcast of the new canonical size
	sendWSResize(t, ctx, conn, 120, 50)
	if cols, rows := readPaneSize(t, ctx, conn); cols != 120 || rows != 50 {
		t.Fatalf("expected broadcast 120x50, got %dx%d", cols, rows)
	}

	// A single client is its own canonical size: shrinking below the previous
	// canonical also broadcasts the new (smaller) size.
	sendWSResize(t, ctx, conn, 40, 10)
	if cols, rows := readPaneSize(t, ctx, conn); cols != 40 || rows != 10 {
		t.Fatalf("expected broadcast 40x10, got %dx%d", cols, rows)
	}

	waitForPaneSize(t, sm, pane.ID, 40, 10)
}

// waitForConPTYSize polls the real ConPTY's tracked size until the debounced
// async resize has actually executed.
func waitForConPTYSize(t *testing.T, pane *Pane, wantCols, wantRows int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pane.PTY == nil {
			t.Fatalf("pane has no ConPTY")
		}
		cols, rows := pane.PTY.GetSize()
		if cols == wantCols && rows == wantRows {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cols, rows := pane.PTY.GetSize()
	t.Fatalf("ConPTY was never resized to %dx%d (still %dx%d)", wantCols, wantRows, cols, rows)
}

// TestWSPane_ResizeActuallyResizesConPTY is a regression test for the bug
// where HandleClientResize/HandleClientDetach committed the canonical size to
// pane.Cols/Rows and then called a duplicate-checking resize that always
// bailed out, so the real ConPTY never changed size (only the Refresh button
// recovered via TriggerForceRedraw).
func TestWSPane_ResizeActuallyResizesConPTY(t *testing.T) {
	sm := NewSessionManager()
	sess, pane, err := sm.CreateSession("prof_resize_test", "ResizeTestSession", "cmd.exe", nil, "", 80, 24)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	defer sm.CloseSession(sess.ID)

	conn := dialPaneWS(t, sm, pane.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if cols, rows := readPaneSize(t, ctx, conn); cols != 80 || rows != 24 {
		t.Fatalf("initial pane-size must be 80x24, got %dx%d", cols, rows)
	}
	if cols, rows := pane.PTY.GetSize(); cols != 80 || rows != 24 {
		t.Fatalf("ConPTY must start at 80x24, got %dx%d", cols, rows)
	}

	// First client grows the canonical size -> the real ConPTY must follow
	sendWSResize(t, ctx, conn, 120, 50)
	if cols, rows := readPaneSize(t, ctx, conn); cols != 120 || rows != 50 {
		t.Fatalf("expected broadcast 120x50, got %dx%d", cols, rows)
	}
	waitForConPTYSize(t, pane, 120, 50)

	// Single client shrinks -> the real ConPTY must follow too
	sendWSResize(t, ctx, conn, 60, 20)
	if cols, rows := readPaneSize(t, ctx, conn); cols != 60 || rows != 20 {
		t.Fatalf("expected broadcast 60x20, got %dx%d", cols, rows)
	}
	waitForConPTYSize(t, pane, 60, 20)

	// A second, larger client attaches and outgrows the first -> ConPTY grows
	c2 := dialPaneWS(t, sm, pane.ID)
	if cols, rows := readPaneSize(t, ctx, c2); cols != 60 || rows != 20 {
		t.Fatalf("c2 initial pane-size must be 60x20, got %dx%d", cols, rows)
	}
	sendWSResize(t, ctx, c2, 100, 40)
	if cols, rows := readPaneSize(t, ctx, c2); cols != 100 || rows != 40 {
		t.Fatalf("expected broadcast 100x40, got %dx%d", cols, rows)
	}
	waitForConPTYSize(t, pane, 100, 40)

	// Largest client (c2) detaches -> canonical shrinks to c1's 60x20 and the
	// real ConPTY must shrink with it
	_ = c2.Close(websocket.StatusNormalClosure, "")
	waitForPaneSize(t, sm, pane.ID, 60, 20)
	waitForConPTYSize(t, pane, 60, 20)
}

// TestCanonicalSize_UnreportedClientDoesNotCollapse guards against a client
// whose viewport has not been reported yet (0x0) collapsing the canonical
// size to zero when every reported client detaches.
func TestCanonicalSize_UnreportedClientDoesNotCollapse(t *testing.T) {
	sm := NewSessionManager()
	pane := newManualPane(sm, "s1", "p1", 100, 40)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c1 := dialPaneWS(t, sm, "p1") // attached but never reports its viewport
	if cols, rows := readPaneSize(t, ctx, c1); cols != 100 || rows != 40 {
		t.Fatalf("c1 initial pane-size must be 100x40, got %dx%d", cols, rows)
	}

	c2 := dialPaneWS(t, sm, "p1")
	if cols, rows := readPaneSize(t, ctx, c2); cols != 100 || rows != 40 {
		t.Fatalf("c2 initial pane-size must be 100x40, got %dx%d", cols, rows)
	}
	sendWSResize(t, ctx, c2, 120, 50)
	waitForPaneSize(t, sm, "p1", 120, 50)

	// The only reported client detaches; c1 (0x0) must NOT collapse the size
	_ = c2.Close(websocket.StatusNormalClosure, "")
	waitForPaneSize(t, sm, "p1", 120, 50)
	pane.mu.Lock()
	cols, rows := pane.Cols, pane.Rows
	pane.mu.Unlock()
	if cols < 10 || rows < 3 {
		t.Fatalf("canonical size collapsed to %dx%d with an unreported client attached", cols, rows)
	}
}
