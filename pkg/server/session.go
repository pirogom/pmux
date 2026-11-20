package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"pmux/pkg/config"
	"pmux/pkg/conpty"
)

// paneEnv returns the current process environment plus WT_SESSION, MSYS, CYGWIN, TERM, and COLORTERM markers.
// Windows Terminal injects WT_SESSION into shells it launches via ConPTY;
// MSYS2's -defterm runtime appears to use its presence to pick a console
// handling path that survives Ctrl+C without losing the ConPTY connection.
// In addition, MSYS=enable_pcon and CYGWIN=enable_pcon ensure MSYS2/Cygwin tools
// (such as MSYS2's OpenSSH /usr/bin/ssh) use ConPTY natively rather than attempting
// to allocate named pipe POSIX pseudo-terminals that hang without terminal output.
// TERM=xterm-256color ensures full-featured ANSI sequence negotiation for ssh, vim, etc.
func paneEnv() []string {
	// Never leak daemon credentials into the shells (and therefore into every
	// program the user runs inside pmux).
	raw := os.Environ()
	env := make([]string, 0, len(raw)+5)
	for _, kv := range raw {
		if strings.HasPrefix(strings.ToUpper(kv), "PMUX_TOKEN=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "WT_SESSION="+uuid.NewString())

	var hasMsys, hasCygwin, hasTerm, hasColorTerm bool
	for _, kv := range env {
		upper := strings.ToUpper(kv)
		if strings.HasPrefix(upper, "MSYS=") {
			hasMsys = true
		} else if strings.HasPrefix(upper, "CYGWIN=") {
			hasCygwin = true
		} else if strings.HasPrefix(upper, "TERM=") {
			hasTerm = true
		} else if strings.HasPrefix(upper, "COLORTERM=") {
			hasColorTerm = true
		}
	}
	if !hasMsys {
		env = append(env, "MSYS=enable_pcon")
	}
	if !hasCygwin {
		env = append(env, "CYGWIN=enable_pcon")
	}
	if !hasTerm {
		env = append(env, "TERM=xterm-256color")
	}
	if !hasColorTerm {
		env = append(env, "COLORTERM=truecolor")
	}
	return env
}

type SplitDirection string

const (
	// SplitHorizontal stacks panes vertically (top/bottom).
	SplitHorizontal SplitDirection = "horizontal"
	// SplitVertical places panes side by side (left/right).
	SplitVertical SplitDirection = "vertical"
)

type LayoutNode struct {
	ID        string         `json:"id"`        // Pane ID (Leaf 노드일 때)
	Direction SplitDirection `json:"direction"` // 분할 방향 (Parent 노드일 때)
	Ratio     float64        `json:"ratio"`     // 분할 비율 (0.5 기본)
	Children  []*LayoutNode  `json:"children"`  // 자식 노드
}

// clientView tracks a single attached client's viewport geometry for a pane.
// The pane keeps ONE canonical terminal size (the largest attached client,
// mirroring tmux's `window-size largest`), so each client only reports its
// own viewport size and the underlying ConPTY is resized only when the
// canonical size itself changes.
type clientView struct {
	conn *websocket.Conn
	ch   chan []byte
	cols int
	rows int
}

type Pane struct {
	ID        string                          `json:"id"`
	SessionID string                          `json:"sessionId"`
	WorkDir   string                          `json:"workDir"`
	Command   string                          `json:"command"`
	Args      []string                        `json:"args"`
	Cols      int                             `json:"cols"`
	Rows      int                             `json:"rows"`
	PTY       *conpty.ConPTY                  `json:"-"`
	Buffer    *RingBuffer                     `json:"-"`
	Clients   map[*websocket.Conn]*clientView `json:"-"`
	// profileEnv holds the profile-specific environment overrides, reused when
	// splitting so child panes behave the same without sharing WT_SESSION.
	profileEnv  []string
	resizeTimer *time.Timer
	inCh        chan []byte
	inDone      chan struct{}
	inOnce      sync.Once
	// overflow holds input chunks that did not fit into inCh. It is bounded so
	// a flood of paste data can never spawn unbounded goroutines or grow
	// memory without limit.
	overflowMu  sync.Mutex
	overflow    [][]byte
	modeTracker *VTModeTracker
	// onExit is invoked when the read loop ends. keepProcessAlive is true when
	// the ConPTY output pipe closed (EOF) while the root process was still
	// STILL_ACTIVE — an anomaly (observed with MSYS2 -defterm shells) rather
	// than a genuine process exit, so the still-running process must not be
	// force-killed.
	onExit func(sessionID, paneID string, keepProcessAlive bool)
	mu     sync.Mutex
}

// maxPaneInputOverflow bounds the per-pane overflow queue (chunks).
const maxPaneInputOverflow = 1024

func newPane(paneID, sessionID, workDir, command string, args []string, cols, rows int, ptyInst *conpty.ConPTY, onExit func(sID, pID string, keepProcessAlive bool)) *Pane {
	p := &Pane{
		ID:          paneID,
		SessionID:   sessionID,
		WorkDir:     workDir,
		Command:     command,
		Args:        args,
		Cols:        cols,
		Rows:        rows,
		PTY:         ptyInst,
		Buffer:      NewRingBuffer(512 * 1024),
		Clients:     make(map[*websocket.Conn]*clientView),
		inCh:        make(chan []byte, 1024),
		inDone:      make(chan struct{}),
		modeTracker: NewVTModeTracker(),
		onExit:      onExit,
	}
	go p.inputLoop()
	return p
}

func (p *Pane) GetModePreamble() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.modeTracker == nil {
		return nil
	}
	return p.modeTracker.ActivePreamble()
}

// WriteInput queues terminal input for the pane. Ordering is preserved: once
// the bounded channel is full, further chunks are appended to an overflow
// queue (also bounded) instead of spawning a goroutine per chunk.
func (p *Pane) WriteInput(data []byte) {
	if len(data) == 0 {
		return
	}
	chunk := make([]byte, len(data))
	copy(chunk, data)

	p.overflowMu.Lock()
	hasOverflow := len(p.overflow) > 0
	if hasOverflow {
		if len(p.overflow) < maxPaneInputOverflow {
			p.overflow = append(p.overflow, chunk)
			p.overflowMu.Unlock()
			return
		}
		p.overflowMu.Unlock()
		return // drop rather than grow without bound
	}
	p.overflowMu.Unlock()

	select {
	case <-p.inDone:
		return
	case p.inCh <- chunk:
	default:
		p.overflowMu.Lock()
		if len(p.overflow) < maxPaneInputOverflow {
			p.overflow = append(p.overflow, chunk)
		}
		p.overflowMu.Unlock()
	}
}

// popOverflow removes and returns the oldest overflow chunk, or nil.
func (p *Pane) popOverflow() []byte {
	p.overflowMu.Lock()
	defer p.overflowMu.Unlock()
	if len(p.overflow) == 0 {
		return nil
	}
	chunk := p.overflow[0]
	p.overflow[0] = nil
	p.overflow = p.overflow[1:]
	return chunk
}

func (p *Pane) inputLoop() {
	write := func(data []byte) {
		p.mu.Lock()
		ptyInst := p.PTY
		p.mu.Unlock()
		if ptyInst != nil && ptyInst.InPipe != nil {
			_, _ = ptyInst.InPipe.Write(data)
		}
	}
	for {
		select {
		case <-p.inDone:
			return
		case data, ok := <-p.inCh:
			if !ok {
				return
			}
			write(data)
			// Drain anything that was queued while the channel was full.
			for {
				next := p.popOverflow()
				if next == nil {
					break
				}
				write(next)
			}
		}
	}
}

func (p *Pane) CloseInput() {
	p.inOnce.Do(func() {
		close(p.inDone)
	})
}

// TriggerForceRedraw records the given size and schedules a full ConPTY
// redraw at it (no duplicate check — callers own change detection).
func (p *Pane) TriggerForceRedraw(cols, rows int) {
	if cols < 10 || rows < 3 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.Cols = cols
	p.Rows = rows
	if p.resizeTimer != nil {
		p.resizeTimer.Stop()
	}

	p.resizeTimer = time.AfterFunc(35*time.Millisecond, func() {
		p.mu.Lock()
		c := p.Cols
		r := p.Rows
		ptyInst := p.PTY
		p.mu.Unlock()

		if ptyInst != nil && c >= 10 && r >= 3 {
			go func(inst *conpty.ConPTY, targetCols, targetRows int) {
				_ = inst.ForceRedraw(targetCols, targetRows)
			}(ptyInst, c, r)
		}
	})
}

// scheduleConptyResize debounces an actual ConPTY resize to the pane's
// current canonical size. Unlike TriggerResize it performs no duplicate
// check: the caller has already committed p.Cols/p.Rows, so comparing them
// again would skip the real resize entirely.
func (p *Pane) scheduleConptyResize() {
	p.mu.Lock()
	defer p.mu.Unlock()

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

// canonicalSize returns the pane's canonical terminal size: the largest
// dimensions reported by any attached client (tmux `window-size largest`
// behavior). Clients that have not reported a viewport yet (0x0) are ignored
// so they cannot collapse the canonical size; when no client has a valid
// viewport the pane's current size is kept. Must be called with p.mu held.
func (p *Pane) canonicalSize() (cols, rows int) {
	hasView := false
	for _, cv := range p.Clients {
		if cv.cols < 10 || cv.rows < 3 {
			continue // viewport not reported yet — leave the size untouched
		}
		hasView = true
		if cv.cols > cols {
			cols = cv.cols
		}
		if cv.rows > rows {
			rows = cv.rows
		}
	}
	if !hasView {
		return p.Cols, p.Rows
	}
	return cols, rows
}

// broadcastPaneSize notifies all attached clients of the pane's canonical
// terminal size. Each client sizes its terminal buffer to this size; clients
// whose viewport is smaller display a clipped viewport into the canonical
// screen (tmux-style), so the raw byte stream stays identical for everyone.
func (p *Pane) broadcastPaneSize(cols, rows int) {
	msg, err := json.Marshal(map[string]interface{}{
		"type": "pane-size",
		"cols": cols,
		"rows": rows,
	})
	if err != nil {
		return
	}
	p.mu.Lock()
	clients := make([]*clientView, 0, len(p.Clients))
	for _, cv := range p.Clients {
		clients = append(clients, cv)
	}
	p.mu.Unlock()

	for _, cv := range clients {
		go func(c *websocket.Conn) {
			_ = safeWriteWS(c, msg)
		}(cv.conn)
	}
}

// HandleClientResize records a single client's viewport size and resizes the
// pane's ConPTY only when the canonical (largest) size changes. Clients
// resizing within the canonical size never trigger a terminal redraw — the
// core fix for multi-client sessions with different window sizes.
// It returns the pane's current canonical size.
func (sm *SessionManager) HandleClientResize(paneID string, conn *websocket.Conn, cols, rows int) (int, int) {
	if cols < 10 || rows < 3 {
		return 0, 0 // Ignore invalid/tiny bounds, mirroring TriggerResize
	}
	pane, ok := sm.GetPane(paneID)
	if !ok {
		return 0, 0
	}

	pane.mu.Lock()
	cv, ok := pane.Clients[conn]
	if !ok {
		// Unknown client (e.g. stale message after detach): report the
		// current canonical size without touching the ConPTY.
		curCols, curRows := pane.Cols, pane.Rows
		pane.mu.Unlock()
		return curCols, curRows
	}
	cv.cols = cols
	cv.rows = rows
	newCols, newRows := pane.canonicalSize()
	changed := newCols != pane.Cols || newRows != pane.Rows
	if changed {
		pane.Cols = newCols
		pane.Rows = newRows
	}
	pane.mu.Unlock()

	if changed {
		pane.scheduleConptyResize()
		pane.broadcastPaneSize(newCols, newRows)
	}
	return newCols, newRows
}

// HandleClientDetach removes a client from the pane and recomputes the
// canonical size. When the largest client leaves, the pane is shrunk to the
// next largest attached client and all remaining clients are notified.
func (sm *SessionManager) HandleClientDetach(paneID string, conn *websocket.Conn) {
	pane, ok := sm.GetPane(paneID)
	if !ok {
		return
	}

	pane.mu.Lock()
	cv, ok := pane.Clients[conn]
	if !ok {
		pane.mu.Unlock()
		return
	}
	delete(pane.Clients, conn)
	close(cv.ch)

	newCols, newRows := pane.canonicalSize()
	changed := newCols != pane.Cols || newRows != pane.Rows
	if changed {
		pane.Cols = newCols
		pane.Rows = newRows
	}
	pane.mu.Unlock()

	if changed {
		pane.scheduleConptyResize()
		pane.broadcastPaneSize(newCols, newRows)
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
	// layoutSeq orders concurrent layout snapshots so an older snapshot can
	// never overwrite a newer one on disk.
	layoutSeq atomic.Uint64
	persistMu sync.Mutex
}

type EventBroadcaster func(eventType string, data interface{})

type SessionManager struct {
	sessions    map[string]*Session
	panes       map[string]*Pane
	broadcaster EventBroadcaster
	orphans     *OrphanTracker
	mu          sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		panes:    make(map[string]*Pane),
		orphans:  NewOrphanTracker(),
	}
}

// ShutdownOrphans terminates any processes left running by the
// forceKill=false path in closePaneInternal, ignoring their grace period.
// Call before the pmux server process exits.
func (sm *SessionManager) ShutdownOrphans() {
	sm.orphans.Shutdown()
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
	sessionID := "sess_" + uuid.NewString()

	if name == "" {
		sm.mu.RLock()
		name = fmt.Sprintf("Session %d", len(sm.sessions)+1)
		sm.mu.RUnlock()
	}

	// 1. Resolve the profile (strictly by ID — the previous name-prefix
	// heuristic could attach a session to the wrong profile).
	var savedLayout *config.SavedLayoutNode
	var matchedProfileID string
	var profileEnv []string
	if profileID != "" {
		if cfg, err := config.LoadConfig(); err == nil && cfg != nil {
			for _, prof := range cfg.Profiles {
				if prof.ID == profileID {
					matchedProfileID = prof.ID
					savedLayout = prof.SavedLayout
					profileEnv = prof.Env
					break
				}
			}
		}
	}
	env := append(paneEnv(), profileEnv...)

	// 2. If a saved layout exists, reconstruct the multi-pane layout. ConPTY
	// creation happens outside the manager lock so a slow shell startup does
	// not block every other session API call.
	if savedLayout != nil && len(savedLayout.Children) > 0 {
		layoutTree, panes, firstPane, errBuild := sm.buildLayoutFromSaved(savedLayout, sessionID, cols, rows, profileEnv)
		if errBuild == nil && layoutTree != nil && firstPane != nil {
			sess := &Session{
				ID:           sessionID,
				Name:         name,
				ProfileID:    matchedProfileID,
				CreatedAt:    time.Now(),
				Panes:        panes,
				ActivePaneID: firstPane.ID,
				Layout:       layoutTree,
			}

			for _, p := range panes {
				p.profileEnv = profileEnv
			}
			sm.mu.Lock()
			sm.sessions[sessionID] = sess
			for id, p := range panes {
				sm.panes[id] = p
			}
			sm.mu.Unlock()

			for _, p := range panes {
				go p.readLoop()
			}
			sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "created"})
			return sess, firstPane, nil
		}
		// Partial failure: make sure no half-built PTY is leaked.
		for _, p := range panes {
			if p.PTY != nil {
				p.PTY.Close()
			}
			p.CloseInput()
		}
	}

	// 3. Default single-pane creation
	paneID := "pane_" + uuid.NewString()
	ptyInst, err := conpty.New(command, args, workDir, env, cols, rows)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ConPTY: %w", err)
	}

	pane := newPane(paneID, sessionID, workDir, command, args, cols, rows, ptyInst, func(sID, pID string, keepProcessAlive bool) {
		_ = sm.closePaneInternal(sID, pID, !keepProcessAlive)
	})
	pane.profileEnv = profileEnv

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

	sm.mu.Lock()
	sm.sessions[sessionID] = sess
	sm.panes[paneID] = pane
	sm.mu.Unlock()

	go pane.readLoop()

	sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "created"})
	return sess, pane, nil
}

// buildLayoutFromSaved materializes a saved layout tree into live panes. It
// does not touch SessionManager state so it can run without holding sm.mu.
func (sm *SessionManager) buildLayoutFromSaved(node *config.SavedLayoutNode, sessionID string, cols, rows int, profileEnv []string) (*LayoutNode, map[string]*Pane, *Pane, error) {
	panes := make(map[string]*Pane)
	layout, firstPane, err := sm.buildLayoutNode(node, sessionID, panes, cols, rows, profileEnv)
	if err != nil {
		return nil, panes, nil, err
	}
	return layout, panes, firstPane, nil
}

func (sm *SessionManager) buildLayoutNode(node *config.SavedLayoutNode, sessionID string, panes map[string]*Pane, cols, rows int, profileEnv []string) (*LayoutNode, *Pane, error) {
	if node == nil {
		return nil, nil, nil
	}

	if len(node.Children) == 0 {
		paneID := "pane_" + uuid.NewString()

		cmd := node.Command
		if cmd == "" {
			cmd = "cmd.exe"
		}

		// Every pane gets its own WT_SESSION marker, matching a fresh split.
		env := append(paneEnv(), profileEnv...)
		ptyInst, err := conpty.New(cmd, node.Args, node.WorkDir, env, cols, rows)
		if err != nil {
			return nil, nil, err
		}

		pane := newPane(paneID, sessionID, node.WorkDir, cmd, node.Args, cols, rows, ptyInst, func(sID, pID string, keepProcessAlive bool) {
			_ = sm.closePaneInternal(sID, pID, !keepProcessAlive)
		})
		panes[paneID] = pane

		return &LayoutNode{ID: paneID}, pane, nil
	}

	var childNodes []*LayoutNode
	var firstPane *Pane

	for _, childSaved := range node.Children {
		childLayout, paneInst, err := sm.buildLayoutNode(childSaved, sessionID, panes, cols, rows, profileEnv)
		if err != nil {
			return nil, nil, err
		}
		if childLayout != nil {
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
	sess, ok := sm.sessions[sessionID]
	if !ok {
		sm.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		sm.mu.Unlock()
		return fmt.Errorf("session name cannot be empty")
	}
	runes := []rune(newName)
	if len(runes) > 256 {
		newName = string(runes[:256])
	}
	sess.Name = newName
	sm.mu.Unlock()

	sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "renamed"})
	return nil
}

func (sm *SessionManager) SplitPane(sessionID, parentPaneID string, direction SplitDirection, command string, args []string, workDir string, cols, rows int) (*Pane, error) {
	// Read the parent's launch parameters under a short read lock, then create
	// the ConPTY without holding the manager lock.
	sm.mu.RLock()
	sess, ok := sm.sessions[sessionID]
	if !ok {
		sm.mu.RUnlock()
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	parentPane, ok := sess.Panes[parentPaneID]
	if !ok {
		sm.mu.RUnlock()
		return nil, fmt.Errorf("parent pane not found: %s", parentPaneID)
	}
	if command == "" {
		command = parentPane.Command
		args = parentPane.Args
	}
	if workDir == "" {
		workDir = parentPane.WorkDir
	}
	profileEnv := parentPane.profileEnv
	sm.mu.RUnlock()
	env := append(paneEnv(), profileEnv...)

	paneID := "pane_split_" + uuid.NewString()

	ptyInst, err := conpty.New(command, args, workDir, env, cols, rows)
	if err != nil {
		return nil, fmt.Errorf("failed to create ConPTY: %w", err)
	}

	newPane := newPane(paneID, sessionID, workDir, command, args, cols, rows, ptyInst, func(sID, pID string, keepProcessAlive bool) {
		_ = sm.closePaneInternal(sID, pID, !keepProcessAlive)
	})
	newPane.profileEnv = profileEnv

	sm.mu.Lock()
	sess, ok = sm.sessions[sessionID]
	if !ok {
		sm.mu.Unlock()
		ptyInst.Close()
		newPane.CloseInput()
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if _, parentStillThere := sess.Panes[parentPaneID]; !parentStillThere {
		sm.mu.Unlock()
		ptyInst.Close()
		newPane.CloseInput()
		return nil, fmt.Errorf("parent pane not found: %s", parentPaneID)
	}

	sess.Panes[paneID] = newPane
	sess.ActivePaneID = paneID
	sm.panes[paneID] = newPane

	// Update Layout Tree
	sess.Layout = insertLayoutNode(sess.Layout, parentPaneID, paneID, direction)

	layoutSnapshot, layoutSeq := persistLayoutLocked(sess)
	sm.mu.Unlock()

	go newPane.readLoop()

	// Persist layout changes outside the lock (config I/O must not block the
	// session API).
	saveLayoutSnapshot(sess, layoutSnapshot, layoutSeq)
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

// stillActiveExitCode is the Win32 STILL_ACTIVE sentinel returned by
// GetExitCodeProcess for a process that has not exited yet.
const stillActiveExitCode = 259

func (p *Pane) readLoop() {
	log.Printf("[conpty server] Starting read loop for pane %s (PID: %d)", p.ID, p.PTY.Pid)
	buf := make([]byte, 65536) // 64KB buffer for fast ANSI sequence draining
	stillActive := false
	for {
		n, err := p.PTY.OutPipe.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			// 1. Instantly write to RingBuffer
			p.Buffer.Write(chunk)

			p.mu.Lock()
			// 2. Track active DECSET/DECRST terminal modes
			if p.modeTracker != nil {
				p.modeTracker.Process(chunk)
			}

			// 3. Non-blocking broadcast to all active client channels. A
			// client that cannot keep up is disconnected instead of having
			// output silently dropped: reconnecting replays the history
			// buffer, which resynchronizes its terminal state.
			var overflowed []*websocket.Conn
			for _, cv := range p.Clients {
				select {
				case cv.ch <- chunk:
				default:
					overflowed = append(overflowed, cv.conn)
				}
			}
			p.mu.Unlock()

			for _, conn := range overflowed {
				_ = conn.Close(websocket.StatusTryAgainLater, "terminal output overflow; resync required")
			}
		}
		if err != nil {
			exitCode, exitErr := p.PTY.ExitCode()
			if exitErr != nil {
				log.Printf("[conpty exit] Pane %s (root PID %d) read loop ended: %v (root exit code lookup failed: %v)", p.ID, p.PTY.Pid, err, exitErr)
			} else {
				log.Printf("[conpty exit] Pane %s (root PID %d) read loop ended: %v (root process exit code: %d)", p.ID, p.PTY.Pid, err, exitCode)
			}
			// The pipe closing does not always mean the process died: MSYS2's
			// -defterm runtime can detach a still-running shell from our
			// pseudo console (root exit code still STILL_ACTIVE=259). In that
			// case don't TerminateProcess a shell that never actually exited.
			stillActive = exitErr == nil && exitCode == stillActiveExitCode
			break
		}
	}

	if p.onExit != nil {
		go p.onExit(p.SessionID, p.ID, stillActive)
	}
}

// cloneLayout deep-copies a layout tree so callers can serialize it without
// holding the session manager lock.
func cloneLayout(n *LayoutNode) *LayoutNode {
	if n == nil {
		return nil
	}
	out := &LayoutNode{
		ID:        n.ID,
		Direction: n.Direction,
		Ratio:     n.Ratio,
	}
	if len(n.Children) > 0 {
		out.Children = make([]*LayoutNode, len(n.Children))
		for i, child := range n.Children {
			out.Children[i] = cloneLayout(child)
		}
	}
	return out
}

// snapshotSessionLocked returns a lock-free copy of a session suitable for
// JSON encoding after sm.mu is released. Copying is required because SplitPane
// and ClosePane mutate the live maps and pane geometry concurrently with HTTP
// handlers encoding the response.
func snapshotSessionLocked(s *Session) *Session {
	panes := make(map[string]*Pane, len(s.Panes))
	for id, p := range s.Panes {
		p.mu.Lock()
		panes[id] = &Pane{
			ID:        p.ID,
			SessionID: p.SessionID,
			WorkDir:   p.WorkDir,
			Command:   p.Command,
			Args:      append([]string(nil), p.Args...),
			Cols:      p.Cols,
			Rows:      p.Rows,
		}
		p.mu.Unlock()
	}
	return &Session{
		ID:           s.ID,
		Name:         s.Name,
		ProfileID:    s.ProfileID,
		CreatedAt:    s.CreatedAt,
		Panes:        panes,
		ActivePaneID: s.ActivePaneID,
		Layout:       cloneLayout(s.Layout),
	}
}

func (sm *SessionManager) ListSessions() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	list := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		list = append(list, snapshotSessionLocked(s))
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

// ClosePane closes a pane and terminates its process. Use for user-initiated
// pane closure, where the underlying process should always be killed.
func (sm *SessionManager) ClosePane(sessionID, paneID string) error {
	return sm.closePaneInternal(sessionID, paneID, true)
}

// closePaneInternal tears down a pane's websocket/session state. When
// forceKill is false, the pane's process is left running instead of being
// terminated — used when the ConPTY pipe closed while the root process was
// still STILL_ACTIVE (see readLoop), so pmux doesn't kill a shell that never
// actually exited.
//
// Only in-memory bookkeeping happens under sm.mu: process termination, socket
// closes and config persistence all run after the lock is released so a slow
// Teardown cannot stall every other session operation.
func (sm *SessionManager) closePaneInternal(sessionID, paneID string, forceKill bool) error {
	sm.mu.Lock()

	sess, ok := sm.sessions[sessionID]
	if !ok {
		sm.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}

	pane, ok := sess.Panes[paneID]
	if !ok {
		sm.mu.Unlock()
		return nil
	}

	pane.CloseInput()
	delete(sess.Panes, paneID)
	delete(sm.panes, paneID)

	pane.mu.Lock()
	clients := pane.Clients
	pane.Clients = make(map[*websocket.Conn]*clientView)
	pane.mu.Unlock()

	// If no panes left in session, close entire session
	sessionClosed := len(sess.Panes) == 0
	var layoutSnapshot *config.SavedLayoutNode
	var layoutSeq uint64
	if sessionClosed {
		delete(sm.sessions, sessionID)
	} else {
		// Update Layout Tree by removing paneID
		sess.Layout = removeLayoutNode(sess.Layout, paneID)

		// Set active pane to any remaining pane if deleted pane was active
		if sess.ActivePaneID == paneID {
			for remainingID := range sess.Panes {
				sess.ActivePaneID = remainingID
				break
			}
		}
		layoutSnapshot, layoutSeq = persistLayoutLocked(sess)
	}
	sm.mu.Unlock()

	if pane.PTY != nil {
		if forceKill {
			pane.PTY.Close()
		} else {
			log.Printf("[conpty exit] Pane %s: root process still active, skipping TerminateProcess", paneID)
			pid := pane.PTY.Pid
			pane.PTY.CloseKeepProcess()
			sm.orphans.Track(pid, sessionID, paneID)
		}
	}

	for conn, cv := range clients {
		close(cv.ch)
		_ = conn.Close(websocket.StatusNormalClosure, "pane closed")
	}

	if !sessionClosed {
		saveLayoutSnapshot(sess, layoutSnapshot, layoutSeq)
		sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "close-pane"})
	}
	return nil
}

func (sm *SessionManager) CloseSession(sessionID string) error {
	sm.mu.Lock()
	sess, ok := sm.sessions[sessionID]
	if !ok {
		sm.mu.Unlock()
		return nil
	}

	type closingPane struct {
		pane    *Pane
		clients map[*websocket.Conn]*clientView
	}
	closing := make([]closingPane, 0, len(sess.Panes))
	for paneID, pane := range sess.Panes {
		pane.CloseInput()
		pane.mu.Lock()
		clients := pane.Clients
		pane.Clients = make(map[*websocket.Conn]*clientView)
		pane.mu.Unlock()
		closing = append(closing, closingPane{pane: pane, clients: clients})
		delete(sm.panes, paneID)
	}

	delete(sm.sessions, sessionID)
	sm.mu.Unlock()

	// Terminate processes and close sockets outside the manager lock.
	for _, cp := range closing {
		if cp.pane.PTY != nil {
			cp.pane.PTY.Close()
		}
		for conn, cv := range cp.clients {
			close(cv.ch)
			_ = conn.Close(websocket.StatusNormalClosure, "session closed")
		}
	}

	sm.notifyChange("session_updated", map[string]interface{}{"sessionId": sessionID, "action": "close-session"})
	return nil
}

// CloseAll terminates every pane and session. It is used on daemon shutdown so
// no ConPTY/root processes are left behind when the server exits.
func (sm *SessionManager) CloseAll() {
	type closingPane struct {
		pane    *Pane
		clients map[*websocket.Conn]*clientView
	}

	sm.mu.Lock()
	closing := make([]closingPane, 0)
	for _, sess := range sm.sessions {
		for paneID, pane := range sess.Panes {
			pane.CloseInput()
			pane.mu.Lock()
			clients := pane.Clients
			pane.Clients = make(map[*websocket.Conn]*clientView)
			pane.mu.Unlock()
			closing = append(closing, closingPane{pane: pane, clients: clients})
			delete(sm.panes, paneID)
		}
	}
	sm.sessions = make(map[string]*Session)
	sm.mu.Unlock()

	for _, cp := range closing {
		if cp.pane.PTY != nil {
			cp.pane.PTY.Close()
		}
		for conn, cv := range cp.clients {
			close(cv.ch)
			_ = conn.Close(websocket.StatusNormalClosure, "server shutting down")
		}
	}
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

// persistLayoutLocked snapshots the session's layout for persistence. It must
// be called with sm.mu held (the layout tree is not safe to read otherwise);
// the returned value and sequence number can then be handed to
// saveLayoutSnapshot after unlocking.
func persistLayoutLocked(sess *Session) (*config.SavedLayoutNode, uint64) {
	if sess == nil || sess.Layout == nil || len(sess.Panes) == 0 {
		return nil, 0 // Never wipe out a profile's saved layout blueprint when session ends or has no panes
	}
	seq := sess.layoutSeq.Add(1)
	return convertToSavedLayoutNode(sess.Layout, sess.Panes), seq
}

// saveLayoutSnapshot persists a layout snapshot unless a newer snapshot has
// already been taken, so concurrent pane closes/splits cannot write an older
// layout over a newer one.
func saveLayoutSnapshot(sess *Session, layout *config.SavedLayoutNode, seq uint64) {
	if sess == nil || layout == nil {
		return
	}
	sess.persistMu.Lock()
	defer sess.persistMu.Unlock()
	if sess.layoutSeq.Load() != seq {
		return // a newer snapshot exists; it will (or did) persist itself
	}
	saveLayoutToProfile(sess.ProfileID, layout)
}

// saveLayoutToProfile persists a layout snapshot to the profile with the given
// ID. Profiles are matched strictly by ID: a renamed session used to silently
// stop persisting because the name-based fallback no longer matched.
func saveLayoutToProfile(profileID string, savedLayout *config.SavedLayoutNode) {
	if profileID == "" || savedLayout == nil {
		return
	}

	updated := false
	err := config.Update(func(cfg *config.Config) error {
		for i := range cfg.Profiles {
			if cfg.Profiles[i].ID == profileID {
				cfg.Profiles[i].SavedLayout = savedLayout
				updated = true
				return nil
			}
		}
		return config.ErrNoChange
	})
	if err != nil {
		log.Printf("[session persistence] failed to save layout for profile %s: %v", profileID, err)
		return
	}
	if updated {
		log.Printf("[session persistence] Saved layout tree to profile (ProfileID: %s)", profileID)
	}
}
