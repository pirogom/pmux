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
