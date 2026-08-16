package ssh

import (
	"bytes"
	"testing"
)

func TestExportImportRoundTrip(t *testing.T) {
	cfg := &Config{
		ClientPath: `C:\tools\ssh.exe`,
		Addresses: []Address{
			{ID: "1", Name: "Prod", Description: "DB server", Host: "10.0.0.1", User: "root"},
		},
	}

	data, err := Export(cfg, "secret-password-123")
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Export returned empty data")
	}
	if bytes.Contains(data, []byte("ssh.exe")) {
		t.Error("export file contains plaintext data")
	}

	imported, err := Import(data, "secret-password-123")
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if imported.ClientPath != cfg.ClientPath {
		t.Errorf("ClientPath mismatch: got %q want %q", imported.ClientPath, cfg.ClientPath)
	}
	if len(imported.Addresses) != 1 {
		t.Fatalf("Address count mismatch: got %d want 1", len(imported.Addresses))
	}
	if imported.Addresses[0].Name != "Prod" || imported.Addresses[0].Host != "10.0.0.1" || imported.Addresses[0].User != "root" {
		t.Errorf("Address mismatch: %+v", imported.Addresses[0])
	}
}

func TestImportWrongPassword(t *testing.T) {
	data, err := Export(Default(), "right-password")
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if _, err := Import(data, "wrong-password"); err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
}

func TestImportCorruptedData(t *testing.T) {
	if _, err := Import([]byte("garbage"), "password"); err == nil {
		t.Fatal("expected error for corrupted export data, got nil")
	}
}

func TestImportUnsupportedVersion(t *testing.T) {
	data := []byte(`{"version":999,"salt":"AA==","data":"AA=="}`)
	if _, err := Import(data, "password"); err == nil {
		t.Fatal("expected ErrUnsupportedVersion for incompatible export, got nil")
	}
}

func TestExportEmptyPassword(t *testing.T) {
	if _, err := Export(Default(), ""); err == nil {
		t.Fatal("expected error for empty export password, got nil")
	}
}
