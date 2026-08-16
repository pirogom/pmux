package ssh

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setTestConfigFile points the package at a temp ssh.conf and restores the
// previous path on cleanup, so tests never touch the real ~/.pmux/ssh.conf.
func setTestConfigFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh.conf")
	old := configFile
	configFile = path
	t.Cleanup(func() { configFile = old })
	return path
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := setTestConfigFile(t)

	cfg := &Config{
		ClientPath: `D:\tools\ssh.exe`,
		Addresses: []Address{
			{ID: "a1", Name: "Prod", Description: "DB server", Host: "10.0.0.1", User: "root"},
			{ID: "a2", Name: "Web", Description: "", Host: "example.com:2222", User: ""},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.ClientPath != cfg.ClientPath {
		t.Errorf("ClientPath mismatch: got %q want %q", loaded.ClientPath, cfg.ClientPath)
	}
	if len(loaded.Addresses) != 2 {
		t.Fatalf("Address count mismatch: got %d want 2", len(loaded.Addresses))
	}
	if loaded.Addresses[0].ID != "a1" || loaded.Addresses[0].Name != "Prod" ||
		loaded.Addresses[0].Description != "DB server" || loaded.Addresses[0].Host != "10.0.0.1" ||
		loaded.Addresses[0].User != "root" {
		t.Errorf("Address[0] mismatch: %+v", loaded.Addresses[0])
	}
	if loaded.Addresses[1].Host != "example.com:2222" || loaded.Addresses[1].User != "" {
		t.Errorf("Address[1] mismatch: %+v", loaded.Addresses[1])
	}

	// The file must not contain plaintext config data (encrypted payload only).
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if strings.Contains(string(data), `D:\tools\ssh.exe`) {
		t.Error("ssh.conf contains plaintext client path")
	}
	if strings.Contains(string(data), `"addresses"`) {
		t.Error("ssh.conf contains plaintext addresses field")
	}
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	setTestConfigFile(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.ClientPath != DefaultClientPath {
		t.Errorf("ClientPath mismatch: got %q want %q", cfg.ClientPath, DefaultClientPath)
	}
	if len(cfg.Addresses) != 0 {
		t.Errorf("expected empty address list, got %d", len(cfg.Addresses))
	}
}

func TestLoadUnsupportedVersion(t *testing.T) {
	path := setTestConfigFile(t)

	content := `{"version":999,"data":"eA=="}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err := Load()
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}

	// The incompatible file must never be overwritten.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != content {
		t.Error("incompatible ssh.conf was modified by Load")
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	setTestConfigFile(t)

	if err := os.WriteFile(configFile, []byte(`{"version":1,"data":"!!not-base64!!"}`), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected error for corrupted file, got nil")
	}
}
