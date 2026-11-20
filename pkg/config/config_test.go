package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain redirects the home directory so the tests never touch the user's
// real ~/.pmux/config.json.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pmux_cfg_test_*")
	if err != nil {
		panic(err)
	}
	origHome, hadHome := os.LookupEnv("HOME")
	origProfile, hadProfile := os.LookupEnv("USERPROFILE")
	_ = os.Setenv("HOME", dir)
	_ = os.Setenv("USERPROFILE", dir)

	code := m.Run()

	if hadHome {
		_ = os.Setenv("HOME", origHome)
	} else {
		_ = os.Unsetenv("HOME")
	}
	if hadProfile {
		_ = os.Setenv("USERPROFILE", origProfile)
	} else {
		_ = os.Unsetenv("USERPROFILE")
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	dir, _ := GetConfigDir()
	t.Logf("Config File Location: %s", filepath.Join(dir, "config.json"))
	t.Logf("Loaded Profiles count: %d", len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		t.Logf("Profile[%d]: ID=%s, Name=%s, Command=%s", i, p.ID, p.Name, p.Command)
	}

	bytes, _ := json.Marshal(cfg)
	t.Logf("Full Config JSON: %s", string(bytes))
}

func TestLoadLegacyConfig(t *testing.T) {
	// Simulate an old config.json without profileFolders / folder fields.
	legacy := `{
  "defaultProfileId": "profile_1",
  "profiles": [
    {"id": "profile_1", "name": "Legacy", "command": "cmd.exe", "args": [], "workDir": ""}
  ],
  "serverPort": 4799,
  "theme": "dark",
  "gitPollInterval": 3
}`
	var cfg Config
	if err := json.Unmarshal([]byte(legacy), &cfg); err != nil {
		t.Fatalf("unmarshal legacy config failed: %v", err)
	}
	if cfg.ProfileFolders == nil {
		cfg.ProfileFolders = []ProfileFolder{}
	}
	if len(cfg.ProfileFolders) != 0 {
		t.Errorf("expected 0 folders for legacy config, got %d", len(cfg.ProfileFolders))
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(cfg.Profiles))
	}
	if cfg.Profiles[0].Folder != "" {
		t.Errorf("expected empty folder for legacy profile, got %q", cfg.Profiles[0].Folder)
	}
}

func testConfig() *Config {
	return &Config{
		Profiles: []Profile{
			{ID: "p1", Name: "P1"},
			{ID: "p2", Name: "P2"},
			{ID: "p3", Name: "P3"},
			{ID: "p4", Name: "P4"},
		},
		ProfileFolders: []ProfileFolder{
			{ID: "f1", Name: "Work"},
			{ID: "f2", Name: "Personal"},
		},
	}
}

func TestCreateFolder(t *testing.T) {
	cfg := testConfig()
	id := cfg.CreateFolder("  My Folder  ")
	if id == "" {
		t.Fatal("expected generated folder id")
	}
	if cfg.ProfileFolders[len(cfg.ProfileFolders)-1].Name != "My Folder" {
		t.Errorf("folder name should be trimmed, got %q", cfg.ProfileFolders[len(cfg.ProfileFolders)-1].Name)
	}
	cfg.CreateFolder("")
	if cfg.ProfileFolders[len(cfg.ProfileFolders)-1].Name != "New Folder" {
		t.Errorf("empty name should default to 'New Folder'")
	}
}

func TestRenameFolder(t *testing.T) {
	cfg := testConfig()
	cfg.RenameFolder("f1", "  Dev  ")
	if cfg.ProfileFolders[0].Name != "Dev" {
		t.Errorf("expected renamed folder to be 'Dev', got %q", cfg.ProfileFolders[0].Name)
	}
	cfg.RenameFolder("does-not-exist", "X")
	if len(cfg.ProfileFolders) != 2 {
		t.Errorf("rename of unknown folder must not change count")
	}
	cfg.RenameFolder("f1", "  ")
	if cfg.ProfileFolders[0].Name != "Dev" {
		t.Errorf("blank name must keep existing name")
	}
}

func TestDeleteFolderMovesProfilesOut(t *testing.T) {
	cfg := testConfig()
	cfg.Profiles[1].Folder = "f1"
	cfg.Profiles[2].Folder = "f1"
	cfg.DeleteFolder("f1")
	if len(cfg.ProfileFolders) != 1 || cfg.ProfileFolders[0].ID != "f2" {
		t.Errorf("folder f1 should be removed, got %+v", cfg.ProfileFolders)
	}
	for _, p := range cfg.Profiles {
		if p.Folder != "" {
			t.Errorf("profile %s should be moved out to root, got folder %q", p.ID, p.Folder)
		}
	}
}

func TestMoveProfile(t *testing.T) {
	folderOrder := func(cfg *Config, folderID string) []string {
		var ids []string
		for _, p := range cfg.Profiles {
			if p.Folder == folderID {
				ids = append(ids, p.ID)
			}
		}
		return ids
	}
	assertOrder := func(t *testing.T, cfg *Config, folderID string, want ...string) {
		t.Helper()
		got := folderOrder(cfg, folderID)
		if len(got) != len(want) {
			t.Fatalf("folder %q members = %v, want %v", folderID, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("folder %q members = %v, want %v", folderID, got, want)
			}
		}
	}

	cfg := testConfig()
	// Move p1 into f1 (empty folder -> becomes first member)
	cfg.MoveProfile("p1", "f1", 0)
	assertOrder(t, cfg, "f1", "p1")
	// Move p2 into f1 after p1
	cfg.MoveProfile("p2", "f1", 1)
	assertOrder(t, cfg, "f1", "p1", "p2")
	// Reorder inside f1: p2 before p1
	cfg.MoveProfile("p2", "f1", 0)
	assertOrder(t, cfg, "f1", "p2", "p1")
	// Move p1 out of f1 to root at the front
	cfg.MoveProfile("p1", "", 0)
	assertOrder(t, cfg, "f1", "p2")
	assertOrder(t, cfg, "", "p1", "p3", "p4")
}

func TestMoveProfileClampsIndex(t *testing.T) {
	cfg := testConfig()
	for i := range cfg.Profiles {
		cfg.Profiles[i].Folder = "f1"
	}
	// Index beyond member count appends at the end.
	cfg.MoveProfile("p4", "f1", 999)
	if cfg.Profiles[3].ID != "p4" {
		t.Errorf("p4 should be at the end, got %+v", cfg.Profiles)
	}
}

func TestMoveProfileUnknownTargets(t *testing.T) {
	cfg := testConfig()
	cfg.MoveProfile("nope", "f1", 0)
	if len(cfg.Profiles) != 4 {
		t.Errorf("unknown profile must not change the list")
	}
	cfg.MoveProfile("p1", "unknown-folder", 0)
	if cfg.Profiles[0].Folder != "" {
		t.Errorf("move to unknown folder must be a no-op")
	}
}

func TestReorderProfileFolders(t *testing.T) {
	cfg := testConfig()
	cfg.ReorderProfileFolders([]string{"f2", "f1"})
	if cfg.ProfileFolders[0].ID != "f2" || cfg.ProfileFolders[1].ID != "f1" {
		t.Errorf("folders should be reordered f2,f1, got %+v", cfg.ProfileFolders)
	}
	// Partial list: unknown ignored, missing keep relative order after known ones.
	cfg2 := testConfig()
	cfg2.ReorderProfileFolders([]string{"nope", "f2"})
	if cfg2.ProfileFolders[0].ID != "f2" || cfg2.ProfileFolders[1].ID != "f1" {
		t.Errorf("partial reorder should give f2,f1, got %+v", cfg2.ProfileFolders)
	}
}

func TestUpdateIsAtomicAndSkipsNoChange(t *testing.T) {
	cfg := &Config{Profiles: []Profile{{ID: "p1", Name: "One"}}, ServerPort: 4799, GitPollInterval: 3}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	if err := Update(func(c *Config) error {
		c.GitPollInterval = 7
		return nil
	}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if got.GitPollInterval != 7 {
		t.Fatalf("expected interval 7, got %d", got.GitPollInterval)
	}

	before, _ := os.ReadFile(mustConfigPath(t))
	if err := Update(func(c *Config) error { return ErrNoChange }); err != nil {
		t.Fatalf("Update with ErrNoChange must not fail: %v", err)
	}
	after, _ := os.ReadFile(mustConfigPath(t))
	if string(before) != string(after) {
		t.Fatalf("ErrNoChange must not rewrite config.json")
	}
}

func TestUpdateCallbackErrorDoesNotWrite(t *testing.T) {
	cfg := &Config{Profiles: []Profile{}, ServerPort: 4799, GitPollInterval: 3}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	before, _ := os.ReadFile(mustConfigPath(t))

	sentinel := errors.New("boom")
	if err := Update(func(c *Config) error {
		c.GitPollInterval = 99
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	after, _ := os.ReadFile(mustConfigPath(t))
	if string(before) != string(after) {
		t.Fatalf("failed Update must not modify config.json")
	}
}

func TestCorruptConfigIsPreservedAndReset(t *testing.T) {
	path := mustConfigPath(t)
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatalf("failed to write corrupt config: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig on corrupt file must recover, got %v", err)
	}
	if cfg.ServerPort != 4799 || cfg.GitPollInterval != 3 {
		t.Fatalf("expected defaults after corruption, got %+v", cfg)
	}

	dir := filepath.Dir(path)
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config.json.corrupt-") {
			found = true
			data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			if string(data) != "{not json" {
				t.Errorf("corrupt backup must preserve original bytes, got %q", data)
			}
		}
	}
	if !found {
		t.Errorf("expected a config.json.corrupt-* backup to be created")
	}
}

func TestGeneratedIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewProfileID()
		if seen[id] {
			t.Fatalf("duplicate profile id generated: %s", id)
		}
		seen[id] = true
	}
	seen = map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewFolderID()
		if seen[id] {
			t.Fatalf("duplicate folder id generated: %s", id)
		}
		seen[id] = true
	}
}

func TestUpsertProfileKeepsSavedLayoutAndFolder(t *testing.T) {
	cfg := &Config{}
	cfg.UpsertProfile(Profile{ID: "p1", Name: "Original", Command: "cmd.exe"})
	cfg.Profiles[0].SavedLayout = &SavedLayoutNode{ID: "pane1"}
	cfg.Profiles[0].Folder = "f1"

	// A partial update (no layout/folder) must not erase them.
	cfg.UpsertProfile(Profile{ID: "p1", Name: "Renamed", Command: "cmd.exe"})
	if cfg.Profiles[0].Name != "Renamed" {
		t.Fatalf("name was not updated: %q", cfg.Profiles[0].Name)
	}
	if cfg.Profiles[0].SavedLayout == nil || cfg.Profiles[0].SavedLayout.ID != "pane1" {
		t.Fatalf("saved layout was erased")
	}
	if cfg.Profiles[0].Folder != "f1" {
		t.Fatalf("folder was erased")
	}

	// A new profile without an ID must get a generated one and not overwrite p1.
	cfg.UpsertProfile(Profile{Name: "New"})
	if len(cfg.Profiles) != 2 || cfg.Profiles[1].ID == "" || cfg.Profiles[1].ID == "p1" {
		t.Fatalf("expected a new profile with a fresh id, got %+v", cfg.Profiles)
	}
}

func TestUpsertProfileKeepsEnv(t *testing.T) {
	cfg := &Config{}
	cfg.UpsertProfile(Profile{
		ID:      "p1",
		Name:    "Original",
		Command: "cmd.exe",
		Env:     []string{"FOO=bar"},
	})

	// The profile editor only sends id/name/command/args/workDir; env must be
	// inherited rather than wiped.
	cfg.UpsertProfile(Profile{ID: "p1", Name: "Renamed", Command: "cmd.exe"})
	if len(cfg.Profiles[0].Env) != 1 || cfg.Profiles[0].Env[0] != "FOO=bar" {
		t.Fatalf("env was erased: %#v", cfg.Profiles[0].Env)
	}
}

func mustConfigPath(t *testing.T) string {
	t.Helper()
	dir, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir failed: %v", err)
	}
	return filepath.Join(dir, "config.json")
}
