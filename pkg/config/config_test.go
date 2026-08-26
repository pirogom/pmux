package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

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
