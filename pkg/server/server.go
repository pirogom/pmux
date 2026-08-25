package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"pmux/pkg/config"
	"pmux/pkg/git"
	"pmux/pkg/opendir"
	"pmux/pkg/ssh"
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
	return safeWriteWSMsg(conn, websocket.MessageText, data)
}

func safeWriteWSBinary(conn *websocket.Conn, data []byte) error {
	return safeWriteWSMsg(conn, websocket.MessageBinary, data)
}

func safeWriteWSMsg(conn *websocket.Conn, msgType websocket.MessageType, data []byte) error {
	if conn == nil {
		return nil
	}
	muVal, _ := wsWriteMu.LoadOrStore(conn, &sync.Mutex{})
	mu := muVal.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return conn.Write(ctx, msgType, data)
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
	mux.HandleFunc("/api/sessions/open-work-folder", s.handleOpenWorkFolder)
	mux.HandleFunc("/api/profiles", s.handleProfiles)
	mux.HandleFunc("/api/profiles/notify-change", s.handleProfilesNotifyChange)
	mux.HandleFunc("/api/git/status", s.handleGitStatus)
	mux.HandleFunc("/api/git/log", s.handleGitLog)
	mux.HandleFunc("/api/git/diff", s.handleGitDiff)
	mux.HandleFunc("/api/git/show", s.handleGitShow)
	mux.HandleFunc("/api/git/branches", s.handleGitBranches)
	mux.HandleFunc("/api/git/remotes", s.handleGitRemotes)
	mux.HandleFunc("/api/git/push", s.handleGitPush)
	mux.HandleFunc("/api/git/pull", s.handleGitPull)
	mux.HandleFunc("/api/git/fetch", s.handleGitFetch)
	mux.HandleFunc("/api/git/commit", s.handleGitCommit)
	mux.HandleFunc("/api/git/stage", s.handleGitStage)
	mux.HandleFunc("/api/git/unstage", s.handleGitUnstage)
	mux.HandleFunc("/api/git/stage-all", s.handleGitStageAll)
	mux.HandleFunc("/api/git/checkout", s.handleGitCheckout)
	mux.HandleFunc("/api/ssh/config", s.handleSSHConfig)
	mux.HandleFunc("/api/ssh/export", s.handleSSHExport)
	mux.HandleFunc("/api/ssh/import", s.handleSSHImport)
	mux.HandleFunc("/api/config/git-poll-interval", s.handleSaveGitPollInterval)
	mux.HandleFunc("/api/config", s.handleGetConfig)
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

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(cfg)
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

// handleOpenWorkFolder opens the given work directory in the OS file manager,
// mirroring the App.OpenWorkFolder Wails binding.
func (s *Server) handleOpenWorkFolder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		WorkDir string `json:"workDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := opendir.Open(req.WorkDir); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
		p.Name = strings.TrimSpace(p.Name)
		runes := []rune(p.Name)
		if len(runes) > 256 {
			p.Name = string(runes[:256])
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

func (s *Server) handleGitLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dir := r.URL.Query().Get("dir")
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	commits, err := git.GetLog(dir, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(commits)
}

func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dir := r.URL.Query().Get("dir")
	path := r.URL.Query().Get("path")
	res := git.GetDiff(dir, path)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGitShow(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dir := r.URL.Query().Get("dir")
	hash := r.URL.Query().Get("hash")
	res := git.GetCommitDetail(dir, hash)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGitBranches(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dir := r.URL.Query().Get("dir")
	branches, err := git.GetBranches(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(branches)
}

func (s *Server) handleGitRemotes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dir := r.URL.Query().Get("dir")
	remotes, err := git.GetRemotes(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(remotes)
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
	_ = json.NewEncoder(w).Encode(res)
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
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGitFetch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		WorkDir string `json:"workDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res := git.Fetch(req.WorkDir)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		WorkDir string `json:"workDir"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res := git.Commit(req.WorkDir, req.Message)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGitStage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		WorkDir string   `json:"workDir"`
		Paths   []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res := git.Stage(req.WorkDir, req.Paths)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGitUnstage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		WorkDir string   `json:"workDir"`
		Paths   []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res := git.Unstage(req.WorkDir, req.Paths)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGitStageAll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		WorkDir string `json:"workDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res := git.StageAll(req.WorkDir)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGitCheckout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		WorkDir string `json:"workDir"`
		Branch  string `json:"branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res := git.Checkout(req.WorkDir, req.Branch)
	_ = json.NewEncoder(w).Encode(res)
}

// handleSSHConfig loads (GET) or saves (POST) the global ssh config,
// mirroring the App.GetSSHConfig / App.SaveSSHConfig Wails bindings so the
// frontend can also work through the daemon HTTP API.
func (s *Server) handleSSHConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost {
		var cfg ssh.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := ssh.Save(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		return
	}

	cfg, err := ssh.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(cfg)
}

// handleSSHExport encrypts the ssh config with the given password and returns
// the export file bytes as base64 (the GUI process opens a save dialog and
// writes the file in the Wails binding path).
func (s *Server) handleSSHExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := ssh.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := ssh.Export(cfg, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"data": base64.StdEncoding.EncodeToString(data)})
}

// handleSSHImport decrypts an uploaded export file (base64) with the given
// password and replaces the current ssh config.
func (s *Server) handleSSHImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
		Data     string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		http.Error(w, "invalid export data", http.StatusBadRequest)
		return
	}
	cfg, err := ssh.Import(data, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := ssh.Save(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleKillServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "server shutting down"})

	// Broadcast shutdown event to all connected GUI clients so they exit
	s.BroadcastEvent("server_shutdown", map[string]string{"reason": "killed by stop/kill command"})

	go func() {
		time.Sleep(500 * time.Millisecond)
		s.sessionMgr.ShutdownOrphans()
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
	paneID := strings.TrimPrefix(r.URL.Path, "/ws/pane/")
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
	conn.SetReadLimit(16 * 1024 * 1024) // 16MB limit for terminal paste/inputs
	defer removeWSConn(conn)

	// Send active VT mode preamble (e.g. Alternate Buffer, SGR Mouse, Bracketed Paste) before history
	preamble := pane.GetModePreamble()
	if len(preamble) > 0 {
		_ = safeWriteWSBinary(conn, preamble)
	}

	// Send history buffer on attach before registering live channel
	history := pane.Buffer.Bytes()
	if len(history) > 0 {
		_ = safeWriteWSBinary(conn, history)
	}

	sendCh := make(chan []byte, 1024)
	pane.mu.Lock()
	pane.Clients[conn] = sendCh
	pane.mu.Unlock()

	defer func() {
		pane.mu.Lock()
		if ch, ok := pane.Clients[conn]; ok {
			delete(pane.Clients, conn)
			close(ch)
		}
		pane.mu.Unlock()
	}()

	// Dedicated non-blocking writer pump: coalesces queued chunks into unified frames
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-sendCh:
				if !ok {
					return
				}
				var buf bytes.Buffer
				buf.Write(chunk)

			drainLoop:
				for buf.Len() < 128*1024 {
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

				if err := safeWriteWSBinary(conn, buf.Bytes()); err != nil {
					cancel()
					return
				}
			}
		}
	}()

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
					pane.WriteInput([]byte(wsMsg.Data))
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
				pane.WriteInput(msg)
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
	conn.SetReadLimit(16 * 1024 * 1024)
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
