package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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
	SavedLayout *SavedLayoutNode `json:"savedLayout,omitempty"`
	Folder      string           `json:"folder,omitempty"`
}

type ProfileFolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Config struct {
	Profiles        []Profile       `json:"profiles"`
	ProfileFolders  []ProfileFolder `json:"profileFolders"`
	ServerPort      int             `json:"serverPort"`
	GitPollInterval int             `json:"gitPollInterval"`
}

// mu serializes config access inside this process. Cross-process access (the
// GUI process and the background daemon both read/write config.json) is
// serialized by an OS file lock (see lock_*.go).
var mu sync.Mutex

// ErrNoChange is returned by an Update callback to signal that nothing was
// modified, so the config file does not need to be rewritten.
var ErrNoChange = errors.New("config: no change")

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

// WithFileLock runs fn while holding an exclusive cross-process lock on
// ~/.pmux/<name>. If the lock file cannot be created/locked (e.g. a read-only
// home directory) fn still runs, protected by the in-process mutex of the
// calling package.
func WithFileLock(name string, fn func() error) error {
	dir, err := GetConfigDir()
	if err != nil {
		return fn()
	}
	lockPath := filepath.Join(dir, name)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fn()
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return fn()
	}
	defer func() { _ = unlockFile(f) }()
	return fn()
}

// withFileLock serializes access to config.json across processes.
func withFileLock(fn func() error) error {
	return WithFileLock("config.lock", fn)
}

func defaultConfig() *Config {
	return &Config{
		ServerPort:      4799,
		GitPollInterval: 3,
		Profiles:        []Profile{},
		ProfileFolders:  []ProfileFolder{},
	}
}

// LoadConfig reads ~/.pmux/config.json. A missing file is created with
// defaults. A corrupt file is preserved as config.json.corrupt-<timestamp> and
// replaced with defaults, so user data is never silently overwritten.
func LoadConfig() (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	var cfg *Config
	err := withFileLock(func() error {
		var e error
		cfg, e = loadLocked()
		return e
	})
	return cfg, err
}

// Update performs an atomic read-modify-write of the config while holding both
// the in-process mutex and the cross-process file lock. The callback must not
// call LoadConfig/SaveConfig. Returning ErrNoChange skips the write.
func Update(fn func(*Config) error) error {
	if fn == nil {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()

	return withFileLock(func() error {
		cfg, err := loadLocked()
		if err != nil {
			return err
		}
		if err := fn(cfg); err != nil {
			if errors.Is(err, ErrNoChange) {
				return nil
			}
			return err
		}
		return saveLocked(cfg)
	})
}

func loadLocked() (*Config, error) {
	configFile, err := getConfigFile()
	if err != nil {
		return nil, err
	}

	cfg := defaultConfig()

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		if err := saveLocked(cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		// Preserve the unparsable file for manual recovery instead of letting
		// the next save destroy it.
		backup := fmt.Sprintf("%s.corrupt-%d", configFile, time.Now().Unix())
		if writeErr := os.WriteFile(backup, data, 0600); writeErr != nil {
			log.Printf("[config] failed to back up corrupt config: %v", writeErr)
		} else {
			log.Printf("[config] config.json was corrupt (%v); saved a copy to %s and reset to defaults", err, backup)
		}
		cfg = defaultConfig()
		if saveErr := saveLocked(cfg); saveErr != nil {
			return cfg, saveErr
		}
		return cfg, nil
	}

	if cfg.GitPollInterval <= 0 {
		cfg.GitPollInterval = 3
	}
	if cfg.ServerPort < 1 || cfg.ServerPort > 65535 {
		cfg.ServerPort = 4799
	}
	if cfg.Profiles == nil {
		cfg.Profiles = []Profile{}
	}
	if cfg.ProfileFolders == nil {
		cfg.ProfileFolders = []ProfileFolder{}
	}

	return cfg, nil
}

func SaveConfig(cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()
	return withFileLock(func() error { return saveLocked(cfg) })
}

// saveLocked writes the config atomically (temp file + rename) so a crash
// during the write can never leave a truncated config.json behind.
func saveLocked(cfg *Config) error {
	if cfg == nil {
		return errors.New("config: nil config")
	}
	configFile, err := getConfigFile()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmp := configFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}

	// os.Rename replaces the destination atomically on Windows only when the
	// target is not open elsewhere; retry briefly before falling back.
	var renameErr error
	for i := 0; i < 5; i++ {
		if renameErr = os.Rename(tmp, configFile); renameErr == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := os.WriteFile(configFile, data, 0600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(tmp)
	return nil
}

func CleanName(name string) string {
	name = strings.TrimSpace(name)
	runes := []rune(name)
	if len(runes) > 256 {
		return string(runes[:256])
	}
	return name
}

// NewProfileID returns a collision-free profile identifier. The previous
// "profile_<len+1>" scheme reused IDs after deletions and silently overwrote
// existing profiles.
func NewProfileID() string {
	return "profile_" + uuid.NewString()
}

// NewFolderID returns a collision-free profile folder identifier.
func NewFolderID() string {
	return "folder_" + uuid.NewString()
}

// UpsertProfile inserts or updates a profile by ID. When updating, an omitted
// saved layout / folder is inherited from the existing entry so partial
// updates from the UI do not erase persisted state.
func (cfg *Config) UpsertProfile(p Profile) {
	p.Name = CleanName(p.Name)

	for i, existing := range cfg.Profiles {
		if p.ID != "" && existing.ID == p.ID {
			if p.SavedLayout == nil && existing.SavedLayout != nil {
				p.SavedLayout = existing.SavedLayout
			}
			if p.Folder == "" && existing.Folder != "" {
				p.Folder = existing.Folder
			}
			// Partial updates from the UI do not include env, so keep the
			// previously configured environment instead of wiping it.
			if p.Env == nil && existing.Env != nil {
				p.Env = existing.Env
			}
			cfg.Profiles[i] = p
			return
		}
	}

	if p.ID == "" {
		p.ID = NewProfileID()
	}
	cfg.Profiles = append(cfg.Profiles, p)
}

// DeleteProfile removes the profile with the given ID.
func (cfg *Config) DeleteProfile(id string) {
	if id == "" {
		return
	}
	newProfiles := make([]Profile, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		if p.ID != id {
			newProfiles = append(newProfiles, p)
		}
	}
	cfg.Profiles = newProfiles
}

// CreateFolder adds a new folder and returns its generated ID.
func (cfg *Config) CreateFolder(name string) string {
	name = CleanName(name)
	if name == "" {
		name = "New Folder"
	}
	id := NewFolderID()
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
