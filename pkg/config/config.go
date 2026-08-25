package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
}

type Config struct {
	DefaultProfileID string    `json:"defaultProfileId"`
	Profiles         []Profile `json:"profiles"`
	ServerPort       int       `json:"serverPort"`
	Theme            string    `json:"theme"`
	GitPollInterval  int       `json:"gitPollInterval"`
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
