package ssh

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"pmux/pkg/config"
	"pmux/pkg/fileutil"
)

// ConfigVersion is the current version of the ssh config file format.
// Data written with a different version is rejected on load (ErrUnsupportedVersion).
const ConfigVersion = 1

// ErrUnsupportedVersion is returned when the ssh.conf file version does not
// match ConfigVersion, meaning the file was written by an incompatible pmux.
var ErrUnsupportedVersion = errors.New("ssh config version is not supported")

// DefaultClientPath is the Windows 10+ built-in OpenSSH client location.
const DefaultClientPath = `C:\Windows\System32\OpenSSH\ssh.exe`

// Address is a single entry of the ssh address book.
type Address struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Host        string `json:"host"`
	User        string `json:"user"`
}

// Config is the ssh settings + address book stored (encrypted) in ~/.pmux/ssh.conf.
type Config struct {
	ClientPath string    `json:"clientPath"`
	Addresses  []Address `json:"addresses"`
}

// envelope is the on-disk format: a version tag plus the DPAPI-encrypted payload.
type envelope struct {
	Version int    `json:"version"`
	Data    string `json:"data"` // base64 of DPAPI-encrypted JSON payload
}

var (
	configFile   string
	configFileMu sync.Mutex
	mu           sync.Mutex
)

// GetConfigFile returns the absolute path of the ssh config file. Tests set
// configFile directly before calling it, so an explicitly assigned path is
// always honored.
func GetConfigFile() (string, error) {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	if configFile != "" {
		return configFile, nil
	}
	dir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	configFile = filepath.Join(dir, "ssh.conf")
	return configFile, nil
}

// Default returns a Config with sane defaults.
func Default() *Config {
	return &Config{
		ClientPath: DefaultClientPath,
		Addresses:  []Address{},
	}
}

// Load reads and decrypts the ssh config. If the file does not exist yet, a
// default config is returned. If the stored version is incompatible,
// ErrUnsupportedVersion is returned and the file is never overwritten.
func Load() (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	cfg := Default()
	path, err := GetConfigFile()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return cfg, fmt.Errorf("failed to parse ssh config: %w", err)
	}

	if env.Version != ConfigVersion {
		return cfg, ErrUnsupportedVersion
	}

	plain, err := decryptData(env.Data)
	if err != nil {
		return cfg, fmt.Errorf("failed to decrypt ssh config: %w", err)
	}

	var loaded Config
	if err := json.Unmarshal(plain, &loaded); err != nil {
		return cfg, fmt.Errorf("failed to parse ssh config payload: %w", err)
	}

	if loaded.ClientPath == "" {
		loaded.ClientPath = DefaultClientPath
	}
	if loaded.Addresses == nil {
		loaded.Addresses = []Address{}
	}
	return &loaded, nil
}

// Save encrypts and writes the ssh config. The GUI process and the background
// daemon can both write it, so a cross-process file lock is taken in addition
// to the in-process mutex.
func Save(cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()
	return config.WithFileLock("ssh.lock", func() error {
		return saveLocked(cfg)
	})
}

func saveLocked(cfg *Config) error {
	if cfg == nil {
		cfg = Default()
	}
	if cfg.Addresses == nil {
		cfg.Addresses = []Address{}
	}

	path, err := GetConfigFile()
	if err != nil {
		return err
	}

	plain, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	enc, err := encryptData(plain)
	if err != nil {
		return err
	}

	env := envelope{Version: ConfigVersion, Data: enc}
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}

	return fileutil.WriteFileAtomic(path, data, 0600)
}
