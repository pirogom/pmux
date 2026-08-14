package server

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRemoveLayoutNode(t *testing.T) {
	sm := NewSessionManager()
	sess, p1, err := sm.CreateSession("prof_1", "TestSession", "cmd.exe", nil, "", 80, 24)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	p2, err := sm.SplitPane(sess.ID, p1.ID, SplitHorizontal, "cmd.exe", nil, "", 80, 24)
	if err != nil {
		t.Fatalf("SplitPane failed: %v", err)
	}

	b1, _ := json.Marshal(sess.Layout)
	t.Logf("Before ClosePane Layout: %s", string(b1))
	t.Logf("Panes count before: %d", len(sess.Panes))

	err = sm.ClosePane(sess.ID, p2.ID)
	if err != nil {
		t.Fatalf("ClosePane failed: %v", err)
	}

	b2, _ := json.Marshal(sess.Layout)
	t.Logf("After ClosePane Layout: %s", string(b2))
	t.Logf("Panes count after: %d", len(sess.Panes))

	if len(sess.Panes) != 1 {
		t.Errorf("Expected 1 pane after close, got %d", len(sess.Panes))
	}
	if sess.Layout.ID != p1.ID {
		t.Errorf("Expected remaining layout ID to be %s, got %s", p1.ID, sess.Layout.ID)
	}
}

func TestPaneExitAutoCleanup(t *testing.T) {
	sm := NewSessionManager()
	sess, p1, err := sm.CreateSession("prof_1", "ExitTestSession", "cmd.exe", []string{"/c", "exit 0"}, "", 80, 24)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	_ = p1

	// Wait up to 3 seconds for cmd.exe to exit and readLoop to trigger auto ClosePane
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		sm.mu.RLock()
		_, exists := sm.sessions[sess.ID]
		sm.mu.RUnlock()
		if !exists {
			t.Logf("Session %s automatically cleaned up on process exit", sess.ID)
			return
		}
	}
	t.Fatalf("Session %s was not cleaned up after process exit", sess.ID)
}
