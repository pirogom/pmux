package server

import (
	"os"
	"path/filepath"
	"testing"

	"pmux/pkg/config"
)

func TestProfileLayoutPersistenceAndRestoration(t *testing.T) {
	// Setup a temporary directory for config testing to avoid modifying user's real ~/.pmux/config.json
	tempDir, err := os.MkdirTemp("", "pmux_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origHome := os.Getenv("USERPROFILE")
	os.Setenv("USERPROFILE", tempDir)
	defer os.Setenv("USERPROFILE", origHome)

	testProfID := "prof_persist_test"
	testProfName := "PersistTestProfile"

	// 1. Initial config with a test profile
	initialCfg := &config.Config{
		Profiles: []config.Profile{
			{
				ID:      testProfID,
				Name:    testProfName,
				Command: "cmd.exe",
				Args:    []string{},
				WorkDir: tempDir,
			},
		},
		ServerPort: 4799,
	}
	if err := config.SaveConfig(initialCfg); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	sm := NewSessionManager()

	// 2. Create Session from profile (Initial: single pane)
	sess1, p1, err := sm.CreateSession(testProfID, testProfName, "cmd.exe", nil, tempDir, 80, 24)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if len(sess1.Panes) != 1 {
		t.Fatalf("Expected 1 pane initially, got %d", len(sess1.Panes))
	}

	// 3. Split pane -> Layout is now 2 panes
	p2, err := sm.SplitPane(sess1.ID, p1.ID, SplitVertical, "cmd.exe", nil, tempDir, 80, 24)
	if err != nil {
		t.Fatalf("SplitPane failed: %v", err)
	}
	_ = p2

	if len(sess1.Panes) != 2 {
		t.Fatalf("Expected 2 panes after split, got %d", len(sess1.Panes))
	}

	// Verify that SavedLayout was saved into config.json
	cfgAfterSplit, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}
	var targetProf *config.Profile
	for _, p := range cfgAfterSplit.Profiles {
		if p.ID == testProfID {
			targetProf = &p
			break
		}
	}
	if targetProf == nil || targetProf.SavedLayout == nil {
		t.Fatalf("SavedLayout was not saved to profile config after SplitPane")
	}
	if len(targetProf.SavedLayout.Children) != 2 {
		t.Fatalf("Expected 2 children in profile SavedLayout, got %d", len(targetProf.SavedLayout.Children))
	}

	// 4. Close the session
	if err := sm.CloseSession(sess1.ID); err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	// Verify that SavedLayout is STILL preserved in config.json and NOT erased to nil
	cfgAfterClose, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to reload config after close: %v", err)
	}
	targetProfAfterClose := (*config.Profile)(nil)
	for _, p := range cfgAfterClose.Profiles {
		if p.ID == testProfID {
			targetProfAfterClose = &p
			break
		}
	}
	if targetProfAfterClose == nil || targetProfAfterClose.SavedLayout == nil {
		t.Fatalf("SavedLayout was erroneously deleted from profile on CloseSession!")
	}
	if len(targetProfAfterClose.SavedLayout.Children) != 2 {
		t.Fatalf("Expected 2 children preserved in SavedLayout, got %d", len(targetProfAfterClose.SavedLayout.Children))
	}

	// 5. Create a NEW session from the same profile -> It must restore 2 panes!
	sess2, firstPane, err := sm.CreateSession(testProfID, testProfName, "cmd.exe", nil, tempDir, 80, 24)
	if err != nil {
		t.Fatalf("CreateSession from saved profile failed: %v", err)
	}
	defer sm.CloseSession(sess2.ID)
	if firstPane == nil {
		t.Fatalf("Expected non-nil firstPane")
	}
	if len(sess2.Panes) != 2 {
		t.Fatalf("Expected 2 panes restored from SavedLayout, got %d", len(sess2.Panes))
	}
	if sess2.Layout == nil || len(sess2.Layout.Children) != 2 {
		t.Fatalf("Expected sess2.Layout to have 2 children, got: %+v", sess2.Layout)
	}

	_ = filepath.Clean(tempDir)
}

// Layouts must only be persisted to a profile matched by ID. The old
// name-prefix fallback could silently attach a layout to the wrong profile.
func TestLayoutPersistenceRequiresProfileID(t *testing.T) {
	cfg := &config.Config{
		Profiles:   []config.Profile{{ID: "p1", Name: "Shell"}},
		ServerPort: 4799,
	}
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// No profile ID, but the session name matches the profile name: must NOT save.
	sess := &Session{
		ID:        "s1",
		Name:      "Shell",
		ProfileID: "",
		Layout:    &LayoutNode{Direction: SplitVertical, Children: []*LayoutNode{{ID: "a"}, {ID: "b"}}},
		Panes: map[string]*Pane{
			"a": {ID: "a", Command: "cmd.exe"},
			"b": {ID: "b", Command: "cmd.exe"},
		},
	}
	layout, seq := persistLayoutLocked(sess)
	saveLayoutSnapshot(sess, layout, seq)

	got, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if got.Profiles[0].SavedLayout != nil {
		t.Fatalf("layout must not be saved without a profile ID")
	}

	// With the ID it is persisted.
	sess.ProfileID = "p1"
	layout, seq = persistLayoutLocked(sess)
	saveLayoutSnapshot(sess, layout, seq)

	got, _ = config.LoadConfig()
	if got.Profiles[0].SavedLayout == nil || len(got.Profiles[0].SavedLayout.Children) != 2 {
		t.Fatalf("expected the layout to be saved by profile ID, got %+v", got.Profiles[0].SavedLayout)
	}
}

// An older snapshot must never overwrite a newer one.
func TestSaveLayoutSnapshotIgnoresStaleSequence(t *testing.T) {
	cfg := &config.Config{Profiles: []config.Profile{{ID: "p1", Name: "Shell"}}, ServerPort: 4799}
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	sess := &Session{
		ID:        "s1",
		ProfileID: "p1",
		Layout:    &LayoutNode{Direction: SplitVertical, Children: []*LayoutNode{{ID: "a"}, {ID: "b"}}},
		Panes: map[string]*Pane{
			"a": {ID: "a", Command: "cmd.exe"},
			"b": {ID: "b", Command: "cmd.exe"},
		},
	}
	oldLayout, oldSeq := persistLayoutLocked(sess)

	// A newer snapshot is taken (and persisted) first.
	sess.Layout = &LayoutNode{Direction: SplitVertical, Children: []*LayoutNode{{ID: "a"}}}
	newLayout, newSeq := persistLayoutLocked(sess)
	saveLayoutSnapshot(sess, newLayout, newSeq)

	// The stale snapshot must be dropped.
	saveLayoutSnapshot(sess, oldLayout, oldSeq)

	got, _ := config.LoadConfig()
	if got.Profiles[0].SavedLayout == nil {
		t.Fatalf("expected a saved layout")
	}
	if len(got.Profiles[0].SavedLayout.Children) != 1 {
		t.Fatalf("stale snapshot overwrote the newer one: %+v", got.Profiles[0].SavedLayout)
	}
}
