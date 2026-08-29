package note

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Note struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

var mu sync.Mutex

func GetNoteDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".pmux", "note")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func hashWorkDir(workDir string) string {
	cleaned := filepath.Clean(workDir)
	sum := sha256.Sum256([]byte(cleaned))
	return hex.EncodeToString(sum[:])
}

func getNoteFile(workDir string) (string, error) {
	dir, err := GetNoteDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, hashWorkDir(workDir)+".json"), nil
}

func Load(workDir string) ([]Note, error) {
	mu.Lock()
	defer mu.Unlock()

	noteFile, err := getNoteFile(workDir)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(noteFile); os.IsNotExist(err) {
		return []Note{}, nil
	}

	data, err := os.ReadFile(noteFile)
	if err != nil {
		return nil, err
	}

	notes := []Note{}
	if err := json.Unmarshal(data, &notes); err != nil {
		return []Note{}, nil
	}
	if notes == nil {
		notes = []Note{}
	}
	return notes, nil
}

func Save(workDir string, notes []Note) error {
	mu.Lock()
	defer mu.Unlock()

	if notes == nil {
		notes = []Note{}
	}
	noteFile, err := getNoteFile(workDir)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(notes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(noteFile, data, 0644)
}
