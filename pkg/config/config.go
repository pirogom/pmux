package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SavedLayoutNode struct {
	ID        string             `json:"id,omitempty"`
	Direction string             `json:"direction,omitempty"` // "horizontal" | "vertical"
	Ratio     float64            `json:"ratio,omitempty"`
	Children  []*SavedLayoutNode `json:"children,omitempty"`
	Command   string             `json:"command,omitempty"`
	Args      []string           `json:"args,omitempty"`
	WorkDir   string             `json:"workDir,omitempty"`
}

type Profile struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Command     string           `json:"command"`
	Args        []string         `json:"args"`
	WorkDir     string           `json:"workDir"`
	Env         []string         `json:"env"`
	IsPreset    bool             `json:"isPreset"`
	SavedLayout *SavedLayoutNode `json:"savedLayout,omitempty"`
	Folder      string           `json:"folder,omitempty"`
}

type ProfileFolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Config struct {
	DefaultProfileID string           `json:"defaultProfileId"`
	Profiles         []Profile        `json:"profiles"`
	ProfileFolders   []ProfileFolder  `json:"profileFolders"`
	ServerPort       int              `json:"serverPort"`
	Theme            string           `json:"theme"`
	GitPollInterval  int              `json:"gitPollInterval"`
}

var mu sync.Mutex

func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".pmux")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func getConfigFile() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func LoadConfig() (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	configFile, err := getConfigFile()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		ServerPort:      4799,
		Theme:           "dark",
		GitPollInterval: 3,
		Profiles:        []Profile{},
		ProfileFolders:  []ProfileFolder{},
	}

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// 기본 설정 생성 및 저장
		SaveConfigLocked(cfg)
		return cfg, nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		if cfg.GitPollInterval <= 0 {
			cfg.GitPollInterval = 3
		}
		return cfg, nil
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		if cfg.GitPollInterval <= 0 {
			cfg.GitPollInterval = 3
		}
		return cfg, nil
	}

	if cfg.GitPollInterval <= 0 {
		cfg.GitPollInterval = 3
	}

	return cfg, nil
}

func SaveConfig(cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()
	return SaveConfigLocked(cfg)
}

func SaveConfigLocked(cfg *Config) error {
	configFile, err := getConfigFile()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configFile, data, 0644)
}

func CleanName(name string) string {
	name = strings.TrimSpace(name)
	runes := []rune(name)
	if len(runes) > 256 {
		return string(runes[:256])
	}
	return name
}

// CreateFolder adds a new folder and returns its generated ID.
func (cfg *Config) CreateFolder(name string) string {
	name = CleanName(name)
	if name == "" {
		name = "New Folder"
	}
	id := fmt.Sprintf("folder_%d", len(cfg.ProfileFolders)+1)
	for i := 0; ; i++ {
		exists := false
		for _, f := range cfg.ProfileFolders {
			if f.ID == id {
				exists = true
				break
			}
		}
		if !exists {
			break
		}
		id = fmt.Sprintf("folder_%d", len(cfg.ProfileFolders)+1+i)
	}
	cfg.ProfileFolders = append(cfg.ProfileFolders, ProfileFolder{ID: id, Name: name})
	return id
}

// RenameFolder renames an existing folder.
func (cfg *Config) RenameFolder(id, name string) {
	name = CleanName(name)
	for i := range cfg.ProfileFolders {
		if cfg.ProfileFolders[i].ID == id {
			if name != "" {
				cfg.ProfileFolders[i].Name = name
			}
			return
		}
	}
}

// DeleteFolder removes a folder; profiles inside it are moved out to the root.
func (cfg *Config) DeleteFolder(id string) {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Folder == id {
			cfg.Profiles[i].Folder = ""
		}
	}
	newFolders := make([]ProfileFolder, 0, len(cfg.ProfileFolders))
	for _, f := range cfg.ProfileFolders {
		if f.ID != id {
			newFolders = append(newFolders, f)
		}
	}
	cfg.ProfileFolders = newFolders
}

// MoveProfile moves a profile to the given folder at the given 0-based index
// (index is relative to the profiles currently inside that folder).
// An empty folderID moves the profile to the root list.
func (cfg *Config) MoveProfile(profileID, folderID string, index int) {
	// Validate target folder exists (empty = root).
	if folderID != "" {
		valid := false
		for _, f := range cfg.ProfileFolders {
			if f.ID == folderID {
				valid = true
				break
			}
		}
		if !valid {
			return
		}
	}

	// Locate and remove the profile.
	profileIdx := -1
	for i := range cfg.Profiles {
		if cfg.Profiles[i].ID == profileID {
			profileIdx = i
			break
		}
	}
	if profileIdx < 0 {
		return
	}
	prof := cfg.Profiles[profileIdx]
	prof.Folder = folderID
	cfg.Profiles = append(cfg.Profiles[:profileIdx], cfg.Profiles[profileIdx+1:]...)

	// Insert before the index-th profile of the target folder.
	count := 0
	insertAt := len(cfg.Profiles)
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Folder == folderID {
			if count == index {
				insertAt = i
				break
			}
			count++
		}
	}
	if count < index {
		insertAt = len(cfg.Profiles)
	}

	cfg.Profiles = append(cfg.Profiles, Profile{})
	copy(cfg.Profiles[insertAt+1:], cfg.Profiles[insertAt:])
	cfg.Profiles[insertAt] = prof
}

// ReorderProfileFolders reorders folders to match the given ID order.
// Unknown IDs are ignored; missing IDs keep their relative positions appended after the known ones.
func (cfg *Config) ReorderProfileFolders(ids []string) {
	ordered := make([]ProfileFolder, 0, len(cfg.ProfileFolders))
	used := make(map[string]bool)
	for _, id := range ids {
		for _, f := range cfg.ProfileFolders {
			if f.ID == id && !used[id] {
				ordered = append(ordered, f)
				used[id] = true
				break
			}
		}
	}
	for _, f := range cfg.ProfileFolders {
		if !used[f.ID] {
			ordered = append(ordered, f)
		}
	}
	cfg.ProfileFolders = ordered
}
