package server

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"pmux/pkg/config"
	"pmux/pkg/conpty"
)

type SplitDirection string

const (
	SplitHorizontal SplitDirection = "horizontal" // 좌우 분할
	SplitVertical   SplitDirection = "vertical"   // 상하 분할
)

type LayoutNode struct {
	ID        string         `json:"id"`        // Pane ID (Leaf 노드일 때)
	Direction SplitDirection `json:"direction"` // 분할 방향 (Parent 노드일 때)
	Ratio     float64        `json:"ratio"`     // 분할 비율 (0.5 기본)
	Children  []*LayoutNode  `json:"children"`  // 자식 노드
}

type Pane struct {
	ID          string                   `json:"id"`
	SessionID   string                   `json:"sessionId"`
	WorkDir     string                   `json:"workDir"`
	Command     string                   `json:"command"`
	Args        []string                 `json:"args"`
	Cols        int                      `json:"cols"`
	Rows        int                      `json:"rows"`
	PTY         *conpty.ConPTY           `json:"-"`
	Buffer      *RingBuffer              `json:"-"`
	Clients     map[*websocket.Conn]bool `json:"-"`
	resizeTimer *time.Timer
	onExit      func(sessionID, paneID string)
	mu          sync.Mutex
}

func (p *Pane) TriggerResize(cols, rows int) {
	if cols < 10 || rows < 3 {
		return // Ignore invalid/tiny bounds to protect ConPTY & TUI apps
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Cols == cols && p.Rows == rows {
		return // Skip duplicate resize
	}

	p.Cols = cols
	p.Rows = rows

	if p.resizeTimer != nil {
		p.resizeTimer.Stop()
	}

	p.resizeTimer = time.AfterFunc(30*time.Millisecond, func() {
		p.mu.Lock()
		c := p.Cols
		r := p.Rows
		ptyInst := p.PTY
		p.mu.Unlock()

		if ptyInst != nil && c >= 10 && r >= 3 {
			// Asynchronous non-blocking Win32 API invocation
			go func(inst *conpty.ConPTY, targetCols, targetRows int) {
				_ = inst.Resize(targetCols, targetRows)
			}(ptyInst, c, r)
		}
	})
}

func (p *Pane) TriggerForceRedraw(cols, rows int) {
	if cols < 10 || rows < 3 {
		return
	}

	p.mu.Lock()
	p.Cols = cols
	p.Rows = rows
	if p.resizeTimer != nil {
		p.resizeTimer.Stop()
	}
	ptyInst := p.PTY
	p.mu.Unlock()

	if ptyInst != nil {
		go func(inst *conpty.ConPTY, targetCols, targetRows int) {
			_ = inst.ForceRedraw(targetCols, targetRows)
		}(ptyInst, cols, rows)
	}
}

type Session struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	ProfileID    string           `json:"profileId"`
	CreatedAt    time.Time        `json:"createdAt"`
	Panes        map[string]*Pane `json:"panes"`
	ActivePaneID string           `json:"activePaneId"`
	Layout       *LayoutNode      `json:"layout"`
	mu           sync.RWMutex
}

type EventBroadcaster func(eventType string, data interface{})

type SessionManager struct {
	sessions    map[string]*Session
	panes       map[string]*Pane
	broadcaster EventBroadcaster
	mu          sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		panes:    make(map[string]*Pane),
	}
}

func (sm *SessionManager) SetBroadcaster(b EventBroadcaster) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.broadcaster = b
}

func (sm *SessionManager) notifyChange(eventType string, data interface{}) {
	if sm.broadcaster != nil {
		go sm.broadcaster(eventType, data)
	}
}

func (sm *SessionManager) CreateSession(profileID, name, command string, args []string, workDir string, cols, rows int) (*Session, *Pane, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now().UnixNano()
	sessionID := fmt.Sprintf("sess_%d", now)

	if name == "" {
		name = fmt.Sprintf("Session %d", len(sm.sessions)+1)
	}

	// 1. Check if profile has a saved layout
	cfg, errConfig := config.LoadConfig()
	var savedLayout *config.SavedLayoutNode
	var matchedProfileID string

	if errConfig == nil && cfg != nil {
		for _, prof := range cfg.Profiles {
			if (profileID != "" && prof.ID == profileID) || prof.ID == name || prof.Name == name || (len(prof.Name) > 0 && strings.HasPrefix(name, prof.Name)) {
				matchedProfileID = prof.ID
				savedLayout = prof.SavedLayout
				break
			}
		}
	}

	// 2. If saved layout exists, reconstruct multi-pane layout
	if savedLayout != nil && len(savedLayout.Children) > 0 {
		sess := &Session{
			ID:        sessionID,
			Name:      name,
			ProfileID: matchedProfileID,
			CreatedAt: time.Now(),
			Panes:     make(map[string]*Pane),
		}
		layoutTree, firstPane, errBuild := sm.buildLayoutFromSaved(savedLayout, sessionID, sess, cols, rows)
		if errBuild == nil && layoutTree != nil && firstPane != nil {
			sess.Layout = layoutTree
			sess.ActivePaneID = firstPane.ID
			sm.sessions[sessionID] = sess
			sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "created"})
			return sess, firstPane, nil
		}
	}

	// 3. Default single-pane creation
	paneID := fmt.Sprintf("pane_%d_1", now)
	ptyInst, err := conpty.New(command, args, workDir, nil, cols, rows)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ConPTY: %w", err)
	}

	pane := &Pane{
		ID:        paneID,
		SessionID: sessionID,
		WorkDir:   workDir,
		Command:   command,
		Args:      args,
		Cols:      cols,
		Rows:      rows,
		PTY:       ptyInst,
		Buffer:    NewRingBuffer(512 * 1024),
		Clients:   make(map[*websocket.Conn]bool),
		onExit: func(sID, pID string) {
			_ = sm.ClosePane(sID, pID)
		},
	}

	sess := &Session{
		ID:           sessionID,
		Name:         name,
		ProfileID:    matchedProfileID,
		CreatedAt:    time.Now(),
		Panes:        map[string]*Pane{paneID: pane},
		ActivePaneID: paneID,
		Layout: &LayoutNode{
			ID: paneID,
		},
	}

	sm.sessions[sessionID] = sess
	sm.panes[paneID] = pane
	go pane.readLoop()

	sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "created"})
	return sess, pane, nil
}

func (sm *SessionManager) buildLayoutFromSaved(node *config.SavedLayoutNode, sessionID string, sess *Session, cols, rows int) (*LayoutNode, *Pane, error) {
	if node == nil {
		return nil, nil, nil
	}

	if node.Children == nil || len(node.Children) == 0 {
		now := time.Now().UnixNano()
		paneID := fmt.Sprintf("pane_%d_%d", now, len(sess.Panes)+1)

		cmd := node.Command
		if cmd == "" {
			cmd = "cmd.exe"
		}

		ptyInst, err := conpty.New(cmd, node.Args, node.WorkDir, nil, cols, rows)
		if err != nil {
			return nil, nil, err
		}

		pane := &Pane{
			ID:        paneID,
			SessionID: sessionID,
			WorkDir:   node.WorkDir,
			Command:   cmd,
			Args:      node.Args,
			Cols:      cols,
			Rows:      rows,
			PTY:       ptyInst,
			Buffer:    NewRingBuffer(512 * 1024),
			Clients:   make(map[*websocket.Conn]bool),
			onExit: func(sID, pID string) {
				_ = sm.ClosePane(sID, pID)
			},
		}

		sess.Panes[paneID] = pane
		sm.panes[paneID] = pane
		go pane.readLoop()

		return &LayoutNode{ID: paneID}, pane, nil
	}

	var childNodes []*LayoutNode
	var firstPane *Pane

	for _, childSaved := range node.Children {
		childLayout, paneInst, err := sm.buildLayoutFromSaved(childSaved, sessionID, sess, cols, rows)
		if err == nil && childLayout != nil {
			childNodes = append(childNodes, childLayout)
			if firstPane == nil && paneInst != nil {
				firstPane = paneInst
			}
		}
	}

	ratio := node.Ratio
	if ratio <= 0 {
		ratio = 0.5
	}

	return &LayoutNode{
		Direction: SplitDirection(node.Direction),
		Ratio:     ratio,
		Children:  childNodes,
	}, firstPane, nil
}

func (sm *SessionManager) RenameSession(sessionID, newName string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sess, ok := sm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("session name cannot be empty")
	}

	sess.Name = newName
	saveLayoutToProfile(sess.ProfileID, sess.Name, sess)
	sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "renamed"})
	return nil
}

func (sm *SessionManager) SplitPane(sessionID, parentPaneID string, direction SplitDirection, command string, args []string, workDir string, cols, rows int) (*Pane, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sess, ok := sm.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	parentPane, ok := sess.Panes[parentPaneID]
	if !ok {
		return nil, fmt.Errorf("parent pane not found: %s", parentPaneID)
	}

	if command == "" {
		command = parentPane.Command
		args = parentPane.Args
	}
	if workDir == "" {
		workDir = parentPane.WorkDir
	}

	paneID := fmt.Sprintf("pane_split_%d", time.Now().UnixNano())

	ptyInst, err := conpty.New(command, args, workDir, nil, cols, rows)
	if err != nil {
		return nil, fmt.Errorf("failed to create ConPTY: %w", err)
	}

	newPane := &Pane{
		ID:        paneID,
		SessionID: sessionID,
		WorkDir:   workDir,
		Command:   command,
		Args:      args,
		Cols:      cols,
		Rows:      rows,
		PTY:       ptyInst,
		Buffer:    NewRingBuffer(512 * 1024),
		Clients:   make(map[*websocket.Conn]bool),
		onExit: func(sID, pID string) {
			_ = sm.ClosePane(sID, pID)
		},
	}

	sess.Panes[paneID] = newPane
	sess.ActivePaneID = paneID
	sm.panes[paneID] = newPane

	// Update Layout Tree
	sess.Layout = insertLayoutNode(sess.Layout, parentPaneID, paneID, direction)

	// Persist layout changes to profile config.json
	saveLayoutToProfile(sess.ProfileID, sess.Name, sess)

	go newPane.readLoop()
	sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "split"})

	return newPane, nil
}

func insertLayoutNode(current *LayoutNode, parentID, newID string, dir SplitDirection) *LayoutNode {
	if current == nil {
		return &LayoutNode{ID: newID}
	}
	if current.ID == parentID {
		// Split this leaf node: inherit target node's ratio for the new parent branch node
		inheritedRatio := current.Ratio
		if inheritedRatio <= 0 {
			inheritedRatio = 1.0
		}
		return &LayoutNode{
			Direction: dir,
			Ratio:     inheritedRatio,
			Children: []*LayoutNode{
				{ID: parentID, Ratio: 0},
				{ID: newID, Ratio: 0},
			},
		}
	}
	for i, child := range current.Children {
		current.Children[i] = insertLayoutNode(child, parentID, newID, dir)
	}
	return current
}

func (p *Pane) readLoop() {
	log.Printf("[conpty server] Starting read loop for pane %s (PID: %d)", p.ID, p.PTY.Pid)
	buf := make([]byte, 32768) // 32KB buffer for fast ANSI sequence draining
	for {
		n, err := p.PTY.OutPipe.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			// 1. Instantly write to RingBuffer
			p.Buffer.Write(chunk)

			// 2. Snapshot active clients quickly under lock & release lock immediately
			p.mu.Lock()
			clients := make([]*websocket.Conn, 0, len(p.Clients))
			for conn := range p.Clients {
				clients = append(clients, conn)
			}
			p.mu.Unlock()

			// 3. Thread-safe broadcast OUTSIDE lock using coder/websocket
			for _, conn := range clients {
				writeCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
				_ = conn.Write(writeCtx, websocket.MessageText, chunk)
				cancel()
			}
		}
		if err != nil {
			log.Printf("[conpty exit] Pane %s read loop ended: %v", p.ID, err)
			break
		}
	}

	if p.onExit != nil {
		go p.onExit(p.SessionID, p.ID)
	}
}

func (sm *SessionManager) ListSessions() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	list := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		list = append(list, s)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].ID < list[j].ID
		}
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})

	return list
}

func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sess, ok := sm.sessions[sessionID]
	return sess, ok
}

func (sm *SessionManager) GetPane(paneID string) (*Pane, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	pane, ok := sm.panes[paneID]
	return pane, ok
}

func (sm *SessionManager) ClosePane(sessionID, paneID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sess, ok := sm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	pane, ok := sess.Panes[paneID]
	if !ok {
		return nil
	}

	if pane.PTY != nil {
		pane.PTY.Close()
	}

	pane.mu.Lock()
	for conn := range pane.Clients {
		_ = conn.Close(websocket.StatusNormalClosure, "pane closed")
	}
	pane.Clients = make(map[*websocket.Conn]bool)
	pane.mu.Unlock()

	delete(sess.Panes, paneID)
	delete(sm.panes, paneID)

	// If no panes left in session, close entire session
	if len(sess.Panes) == 0 {
		delete(sm.sessions, sessionID)
		return nil
	}

	// Update Layout Tree by removing paneID
	sess.Layout = removeLayoutNode(sess.Layout, paneID)

	// Set active pane to any remaining pane if deleted pane was active
	if sess.ActivePaneID == paneID {
		for remainingID := range sess.Panes {
			sess.ActivePaneID = remainingID
			break
		}
	}

	// Persist layout changes to profile config.json
	saveLayoutToProfile(sess.ProfileID, sess.Name, sess)

	sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "close-pane"})
	return nil
}

func (sm *SessionManager) CloseSession(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sess, ok := sm.sessions[sessionID]
	if !ok {
		return nil
	}

	for paneID, pane := range sess.Panes {
		if pane.PTY != nil {
			pane.PTY.Close()
		}
		pane.mu.Lock()
		for conn := range pane.Clients {
			_ = conn.Close(websocket.StatusNormalClosure, "session closed")
		}
		pane.Clients = make(map[*websocket.Conn]bool)
		pane.mu.Unlock()
		delete(sm.panes, paneID)
	}

	delete(sm.sessions, sessionID)
	sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "close-session"})
	return nil
}

func (sm *SessionManager) SetActivePane(sessionID, paneID string) error {
	sm.mu.Lock()
	sess, ok := sm.sessions[sessionID]
	if !ok {
		sm.mu.Unlock()
		return fmt.Errorf("session %s not found", sessionID)
	}
	if _, ok := sess.Panes[paneID]; !ok {
		sm.mu.Unlock()
		return fmt.Errorf("pane %s not found in session %s", paneID, sessionID)
	}
	if sess.ActivePaneID == paneID {
		sm.mu.Unlock()
		return nil
	}
	sess.ActivePaneID = paneID
	sm.mu.Unlock()

	sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "paneId": paneID, "action": "active-pane"})
	return nil
}

func removeLayoutNode(current *LayoutNode, targetID string) *LayoutNode {
	if current == nil {
		return nil
	}
	// Leaf Node
	if current.ID != "" {
		if current.ID == targetID {
			return nil
		}
		return current
	}

	// Branch Node
	var newChildren []*LayoutNode
	for _, child := range current.Children {
		cleaned := removeLayoutNode(child, targetID)
		if cleaned != nil {
			newChildren = append(newChildren, cleaned)
		}
	}

	if len(newChildren) == 0 {
		return nil
	}
	if len(newChildren) == 1 {
		// Single child remaining -> Collapse branch node and promote child
		return newChildren[0]
	}

	current.Children = newChildren
	return current
}



func convertToSavedLayoutNode(node *LayoutNode, panes map[string]*Pane) *config.SavedLayoutNode {
	if node == nil {
		return nil
	}
	if node.ID != "" {
		pane := panes[node.ID]
		if pane != nil {
			return &config.SavedLayoutNode{
				ID:      node.ID,
				Command: pane.Command,
				Args:    pane.Args,
				WorkDir: pane.WorkDir,
			}
		}
		return &config.SavedLayoutNode{ID: node.ID}
	}

	var children []*config.SavedLayoutNode
	for _, child := range node.Children {
		children = append(children, convertToSavedLayoutNode(child, panes))
	}

	return &config.SavedLayoutNode{
		Direction: string(node.Direction),
		Ratio:     node.Ratio,
		Children:  children,
	}
}

func saveLayoutToProfile(profileID, sessionName string, sess *Session) {
	if sess == nil || sess.Layout == nil || len(sess.Panes) == 0 {
		return // Never wipe out a profile's saved layout blueprint when session ends or has no panes
	}

	cfg, err := config.LoadConfig()
	if err != nil || cfg == nil {
		return
	}

	savedLayout := convertToSavedLayoutNode(sess.Layout, sess.Panes)
	if savedLayout == nil {
		return
	}

	updated := false
	for i, prof := range cfg.Profiles {
		if (profileID != "" && prof.ID == profileID) || prof.Name == sessionName || (len(prof.Name) > 0 && strings.HasPrefix(sessionName, prof.Name)) {
			cfg.Profiles[i].SavedLayout = savedLayout
			updated = true
			break
		}
	}

	if updated {
		_ = config.SaveConfig(cfg)
		log.Printf("[session persistence] Saved layout tree to profile %s (ProfileID: %s)", sessionName, profileID)
	}
}
