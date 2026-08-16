package server

import (
	"encoding/json"
	"testing"
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

func TestPane_AsyncInputPipeline(t *testing.T) {
	sm := NewSessionManager()
	sess, pane, err := sm.CreateSession("prof_input_test", "InputTestSession", "cmd.exe", nil, "", 80, 24)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	defer sm.CloseSession(sess.ID)

	// Simulate rapid burst of 2000 keystrokes / long paste chunks
	for i := 0; i < 2000; i++ {
		pane.WriteInput([]byte("a"))
	}
	pane.WriteInput([]byte("\r\n"))
}
