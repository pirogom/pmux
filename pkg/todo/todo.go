package todo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"pmux/pkg/fileutil"
)

type Item struct {
	ID            string `json:"id"`
	Content       string `json:"content"`
	Done          bool   `json:"done"`
	Strikethrough bool   `json:"strikethrough"`
}

var mu sync.Mutex

func GetTodoDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".pmux", "todo")
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

func getTodoFile(workDir string) (string, error) {
	dir, err := GetTodoDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, hashWorkDir(workDir)+".json"), nil
}

func Load(workDir string) ([]Item, error) {
	mu.Lock()
	defer mu.Unlock()

	todoFile, err := getTodoFile(workDir)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(todoFile); os.IsNotExist(err) {
		return []Item{}, nil
	}

	data, err := os.ReadFile(todoFile)
	if err != nil {
		return nil, err
	}

	items := []Item{}
	if err := json.Unmarshal(data, &items); err != nil {
		// Never silently discard a user's todo list: keep a copy for manual
		// recovery before starting over with an empty list.
		if backup, berr := fileutil.BackupCorrupt(todoFile, data); berr == nil {
			log.Printf("[todo] corrupt todo file (%v); backup saved to %s", err, backup)
		}
		return []Item{}, nil
	}
	if items == nil {
		items = []Item{}
	}
	return items, nil
}

func Save(workDir string, items []Item) error {
	mu.Lock()
	defer mu.Unlock()

	if items == nil {
		items = []Item{}
	}
	todoFile, err := getTodoFile(workDir)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(todoFile, data, 0600)
}
