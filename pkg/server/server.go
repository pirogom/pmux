package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"pmux/pkg/config"
	"pmux/pkg/git"
)

type Server struct {
	port         int
	sessionMgr   *SessionManager
	srv          *http.Server
	eventClients map[*websocket.Conn]bool
	eventMu      sync.Mutex
	mu           sync.Mutex
}

func NewServer(port int) *Server {
	if port <= 0 {
		port = 4799
	}
	s := &Server{
		port:         port,
		sessionMgr:   NewSessionManager(),
		eventClients: make(map[*websocket.Conn]bool),
	}
	s.sessionMgr.SetBroadcaster(s.BroadcastEvent)
	return s
}

var wsWriteMu sync.Map // map[*websocket.Conn]*sync.Mutex

func safeWriteWS(conn *websocket.Conn, data []byte) error {
	if conn == nil {
		return nil
	}
	muVal, _ := wsWriteMu.LoadOrStore(conn, &sync.Mutex{})
	mu := muVal.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, data)
}

func removeWSConn(conn *websocket.Conn) {
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
		wsWriteMu.Delete(conn)
	}
}

func (s *Server) BroadcastEvent(eventType string, data interface{}) {
	msg, err := json.Marshal(map[string]interface{}{
		"type": eventType,
		"data": data,
	})
	if err != nil {
		return
	}

	s.eventMu.Lock()
	clients := make([]*websocket.Conn, 0, len(s.eventClients))
	for conn := range s.eventClients {
		clients = append(clients, conn)
	}
	s.eventMu.Unlock()

	for _, conn := range clients {
		go func(c *websocket.Conn) {
			_ = safeWriteWS(c, msg)
		}(conn)
	}
}

func (s *Server) GetSessionManager() *SessionManager {
	return s.sessionMgr
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/sessions/split", s.handleSplit)
	mux.HandleFunc("/api/sessions/close-pane", s.handleClosePane)
	mux.HandleFunc("/api/sessions/close-session", s.handleCloseSession)
	mux.HandleFunc("/api/sessions/rename", s.handleRenameSession)
	mux.HandleFunc("/api/sessions/active-pane", s.handleSetActivePane)
	mux.HandleFunc("/api/profiles", s.handleProfiles)
	mux.HandleFunc("/api/profiles/notify-change", s.handleProfilesNotifyChange)
	mux.HandleFunc("/api/git/status", s.handleGitStatus)
	mux.HandleFunc("/api/git/push", s.handleGitPush)
	mux.HandleFunc("/api/git/pull", s.handleGitPull)
	mux.HandleFunc("/api/config/git-poll-interval", s.handleSaveGitPollInterval)
	mux.HandleFunc("/api/server/kill", s.handleKillServer)
	mux.HandleFunc("/ws/pane/", s.handleWSPane)
	mux.HandleFunc("/ws/events", s.handleWSEvents)

	s.srv = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: corsMiddleware(mux),
	}

	log.Printf("[pmux server] Started listening on http://127.0.0.1:%d", s.port)
	return s.srv.ListenAndServe()
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		sessions := s.sessionMgr.ListSessions()
		_ = json.NewEncoder(w).Encode(sessions)
	case http.MethodPost:
		var req struct {
			ProfileID string   `json:"profileId"`
			Name      string   `json:"name"`
			Command   string   `json:"command"`
			Args      []string `json:"args"`
			WorkDir   string   `json:"workDir"`
			Cols      int      `json:"cols"`
			Rows      int      `json:"rows"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		sess, pane, err := s.sessionMgr.CreateSession(req.ProfileID, req.Name, req.Command, req.Args, req.WorkDir, req.Cols, req.Rows)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"session": sess,
			"pane":    pane,
		})
	}
}

func (s *Server) handleSplit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID    string         `json:"sessionId"`
		ParentPaneID string         `json:"parentPaneId"`
		Direction    SplitDirection `json:"direction"`
		Command      string         `json:"command"`
		Args         []string       `json:"args"`
		WorkDir      string         `json:"workDir"`
		Cols         int            `json:"cols"`
		Rows         int            `json:"rows"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pane, err := s.sessionMgr.SplitPane(req.SessionID, req.ParentPaneID, req.Direction, req.Command, req.Args, req.WorkDir, req.Cols, req.Rows)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pane)
}

func (s *Server) handleSaveGitPollInterval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Interval int `json:"interval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Interval < 1 {
		req.Interval = 1
	} else if req.Interval > 10 {
		req.Interval = 10
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.GitPollInterval = req.Interval
	if err := config.SaveConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.BroadcastEvent("config_updated", map[string]interface{}{"gitPollInterval": req.Interval})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleClosePane(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"sessionId"`
		PaneID    string `json:"paneId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := s.sessionMgr.ClosePane(req.SessionID, req.PaneID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleRenameSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"sessionId"`
		NewName   string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := s.sessionMgr.RenameSession(req.SessionID, req.NewName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleCloseSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := s.sessionMgr.CloseSession(req.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleSetActivePane(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"sessionId"`
		PaneID    string `json:"paneId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
		_ = s.sessionMgr.SetActivePane(req.SessionID, req.PaneID)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleProfilesNotifyChange(w http.ResponseWriter, r *http.Request) {
	s.BroadcastEvent("profiles_updated", nil)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}



func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost {
		var p config.Profile
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg, err := config.LoadConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		found := false
		for i, existing := range cfg.Profiles {
			if existing.ID == p.ID {
				if p.SavedLayout == nil && existing.SavedLayout != nil {
					p.SavedLayout = existing.SavedLayout
				}
				cfg.Profiles[i] = p
				found = true
				break
			}
		}
		if !found {
			if p.ID == "" {
				p.ID = fmt.Sprintf("profile_%d", len(cfg.Profiles)+1)
			}
			cfg.Profiles = append(cfg.Profiles, p)
		}
		if err := config.SaveConfig(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.BroadcastEvent("profiles_updated", nil)
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		return
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(cfg.Profiles)
}

func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dir := r.URL.Query().Get("dir")
	res := git.GetStatus(dir)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGitPush(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		WorkDir string `json:"workDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res := git.Push(req.WorkDir)
	_ = json.NewEncoder(w).Encode(map[string]string{"output": res})
}

func (s *Server) handleGitPull(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		WorkDir string `json:"workDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res := git.Pull(req.WorkDir)
	_ = json.NewEncoder(w).Encode(map[string]string{"output": res})
}

func (s *Server) handleKillServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "server shutting down"})

	// Broadcast shutdown event to all connected GUI clients so they exit
	s.BroadcastEvent("server_shutdown", map[string]string{"reason": "killed by stop/kill command"})

	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

type WSIncomingMsg struct {
	Type string `json:"type"` // "input" or "resize"
	Data string `json:"data"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func (s *Server) handleWSPane(w http.ResponseWriter, r *http.Request) {
	paneID := r.URL.Path[len("/ws/pane/"):]
	pane, ok := s.sessionMgr.GetPane(paneID)
	if !ok {
		log.Printf("[ws 404] Pane not found: %s", paneID)
		http.Error(w, "Pane not found", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("WS Accept error for pane %s: %v", paneID, err)
		return
	}
	defer removeWSConn(conn)

	pane.mu.Lock()
	pane.Clients[conn] = true
	pane.mu.Unlock()

	defer func() {
		pane.mu.Lock()
		delete(pane.Clients, conn)
		pane.mu.Unlock()
	}()

	// Send history buffer on attach!
	history := pane.Buffer.Bytes()
	if len(history) > 0 {
		_ = safeWriteWS(conn, history)
	}

	// Incoming message loop (Inputs & Resizes)
	for {
		mt, msg, err := conn.Read(r.Context())
		if err != nil {
			break
		}
		if mt == websocket.MessageText {
			var wsMsg WSIncomingMsg
			if err := json.Unmarshal(msg, &wsMsg); err == nil {
				switch wsMsg.Type {
				case "input":
					if pane.PTY != nil && pane.PTY.InPipe != nil {
						_, _ = pane.PTY.InPipe.Write([]byte(wsMsg.Data))
					}
				case "resize":
					if pane.PTY != nil && wsMsg.Cols > 0 && wsMsg.Rows > 0 {
						pane.TriggerResize(wsMsg.Cols, wsMsg.Rows)
					}
				case "redraw":
					if pane.PTY != nil && wsMsg.Cols > 0 && wsMsg.Rows > 0 {
						pane.TriggerForceRedraw(wsMsg.Cols, wsMsg.Rows)
					}
				}
			} else {
				// Raw string input fallback
				if pane.PTY != nil && pane.PTY.InPipe != nil {
					_, _ = pane.PTY.InPipe.Write(msg)
				}
			}
		}
	}
}

func (s *Server) handleWSEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("WS Event Accept error: %v", err)
		return
	}
	defer removeWSConn(conn)

	s.eventMu.Lock()
	s.eventClients[conn] = true
	s.eventMu.Unlock()

	defer func() {
		s.eventMu.Lock()
		delete(s.eventClients, conn)
		s.eventMu.Unlock()
	}()

	for {
		if _, _, err := conn.Read(r.Context()); err != nil {
			break
		}
	}
}

func GetServerPort() int {
	cfg, err := config.LoadConfig()
	if err != nil || cfg.ServerPort <= 0 {
		return 4799
	}
	return cfg.ServerPort
}

func IsServerRunning(port int) bool {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

var (
	defaultServer *Server
	serverOnce    sync.Once
)

func GetDefaultServer(port int) *Server {
	serverOnce.Do(func() {
		defaultServer = NewServer(port)
	})
	return defaultServer
}

func EnsureServerRunning(port int) error {
	if IsServerRunning(port) {
		return nil
	}

	// Spawn independent background server daemon process
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	cmd := exec.Command(execPath, "--server", "--port", strconv.Itoa(port))
	prepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start background server daemon: %w", err)
	}

	// Wait up to 3 seconds for server daemon to respond
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if IsServerRunning(port) {
			return nil
		}
	}

	return fmt.Errorf("background server daemon failed to start on port %d", port)
}
