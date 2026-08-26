package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"pmux/pkg/config"
	"pmux/pkg/git"
	"pmux/pkg/opendir"
	"pmux/pkg/profile"
	"pmux/pkg/server"
	"pmux/pkg/ssh"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 1. Load config
	_, _ = config.LoadConfig()

	// 2. Ensure background server is running
	port := server.GetServerPort()
	_ = server.EnsureServerRunning(port)
}

func (a *App) GetConfig() (*config.Config, error) {
	return config.LoadConfig()
}

func (a *App) RenameSession(sessionID, newName string) error {
	port := server.GetServerPort()
	_ = server.EnsureServerRunning(port)

	req := map[string]string{
		"sessionId": sessionID,
		"newName":   newName,
	}
	bodyData, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/api/sessions/rename", port), "application/json", bytes.NewBuffer(bodyData))
	if err != nil {
		return fmt.Errorf("failed to rename session: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (a *App) SaveGitPollInterval(interval int) error {
	if interval < 1 {
		interval = 1
	} else if interval > 10 {
		interval = 10
	}
	port := server.GetServerPort()
	_ = server.EnsureServerRunning(port)

	bodyData, err := json.Marshal(map[string]int{"interval": interval})
	if err != nil {
		return err
	}

	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/api/config/git-poll-interval", port), "application/json", bytes.NewBuffer(bodyData))
	if err != nil {
		return fmt.Errorf("failed to save git poll interval: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (a *App) SaveProfile(p config.Profile) error {
	p.Name = strings.TrimSpace(p.Name)
	runes := []rune(p.Name)
	if len(runes) > 256 {
		p.Name = string(runes[:256])
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	found := false
	for i, existing := range cfg.Profiles {
		if existing.ID == p.ID {
			if p.SavedLayout == nil && existing.SavedLayout != nil {
				p.SavedLayout = existing.SavedLayout
			}
			if p.Folder == "" && existing.Folder != "" {
				p.Folder = existing.Folder
			}
			cfg.Profiles[i] = p
			found = true
			break
		}
	}
	if !found {
		if p.ID == "" {
			p.ID = fmt.Sprintf("profile_%d", len(cfg.Profiles)+1)
		}
		cfg.Profiles = append(cfg.Profiles, p)
	}
	err = config.SaveConfig(cfg)
	if err == nil {
		a.notifyProfilesChanged()
	}
	return err
}

func (a *App) DeleteProfile(id string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	newProfiles := make([]config.Profile, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		if p.ID != id {
			newProfiles = append(newProfiles, p)
		}
	}
	cfg.Profiles = newProfiles
	err = config.SaveConfig(cfg)
	if err == nil {
		a.notifyProfilesChanged()
	}
	return err
}

func (a *App) notifyProfilesChanged() {
	port := server.GetServerPort()
	_, _ = http.Post(fmt.Sprintf("http://127.0.0.1:%d/api/profiles/notify-change", port), "application/json", nil)
}

func (a *App) CreateProfileFolder(name string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.CreateFolder(name)
	err = config.SaveConfig(cfg)
	if err == nil {
		a.notifyProfilesChanged()
	}
	return err
}

func (a *App) RenameProfileFolder(id, name string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.RenameFolder(id, name)
	err = config.SaveConfig(cfg)
	if err == nil {
		a.notifyProfilesChanged()
	}
	return err
}

func (a *App) DeleteProfileFolder(id string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.DeleteFolder(id)
	err = config.SaveConfig(cfg)
	if err == nil {
		a.notifyProfilesChanged()
	}
	return err
}

func (a *App) MoveProfile(profileID, folderID string, index int) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.MoveProfile(profileID, folderID, index)
	err = config.SaveConfig(cfg)
	if err == nil {
		a.notifyProfilesChanged()
	}
	return err
}

func (a *App) ReorderProfileFolders(ids []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.ReorderProfileFolders(ids)
	err = config.SaveConfig(cfg)
	if err == nil {
		a.notifyProfilesChanged()
	}
	return err
}

func (a *App) GetProfiles() ([]config.Profile, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	return cfg.Profiles, nil
}

func (a *App) GetDetectedProfiles() []config.Profile {
	return profile.DetectProfiles()
}

func (a *App) GetServerPort() int {
	return server.GetServerPort()
}

func (a *App) GetGitStatus(workDir string) git.GitStatusResult {
	return git.GetStatus(workDir)
}

func (a *App) GitPush(workDir string) git.GitOpResult {
	return git.Push(workDir)
}

func (a *App) GitPull(workDir string) git.GitOpResult {
	return git.Pull(workDir)
}

func (a *App) GitFetch(workDir string) git.GitOpResult {
	return git.Fetch(workDir)
}

func (a *App) GitCommit(workDir, message string) git.GitOpResult {
	return git.Commit(workDir, message)
}

func (a *App) GitStage(workDir string, paths []string) git.GitOpResult {
	return git.Stage(workDir, paths)
}

func (a *App) GitUnstage(workDir string, paths []string) git.GitOpResult {
	return git.Unstage(workDir, paths)
}

func (a *App) GitStageAll(workDir string) git.GitOpResult {
	return git.StageAll(workDir)
}

func (a *App) GetGitLog(workDir string, limit int) ([]git.GitCommit, error) {
	return git.GetLog(workDir, limit)
}

func (a *App) GetGitDiff(workDir, path string) git.GitDiffResult {
	return git.GetDiff(workDir, path)
}

func (a *App) GetGitCommitDetail(workDir, hash string) git.GitCommitDetail {
	return git.GetCommitDetail(workDir, hash)
}

func (a *App) GetGitBranches(workDir string) ([]git.GitBranch, error) {
	return git.GetBranches(workDir)
}

func (a *App) GitCheckout(workDir, branch string) git.GitOpResult {
	return git.Checkout(workDir, branch)
}

func (a *App) GetGitRemotes(workDir string) ([]git.GitRemote, error) {
	return git.GetRemotes(workDir)
}

func (a *App) KillServer() error {
	port := server.GetServerPort()
	if server.IsServerRunning(port) {
		_, _ = http.Post(fmt.Sprintf("http://127.0.0.1:%d/api/server/kill", port), "application/json", nil)
	}
	return nil
}

// Session API Bindings for Frontend
type CreateSessionReq struct {
	ProfileID string   `json:"profileId"`
	Name      string   `json:"name"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	WorkDir   string   `json:"workDir"`
	Cols      int      `json:"cols"`
	Rows      int      `json:"rows"`
}

func (a *App) CreateSession(req CreateSessionReq) (map[string]interface{}, error) {
	port := server.GetServerPort()
	_ = server.EnsureServerRunning(port)

	// Send HTTP POST to the actual listening 4799 server instance to guarantee Session & WS port alignment!
	bodyData, err := json.Marshal(req)
	if err != nil {
		fmt.Printf("[pmux err] Marshal error: %v\n", err)
		return nil, err
	}

	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/api/sessions", port), "application/json", bytes.NewBuffer(bodyData))
	if err != nil {
		fmt.Printf("[pmux err] CreateSession HTTP failed: %v\n", err)
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Printf("[pmux err] Decode session response failed: %v\n", err)
		return nil, err
	}
	return result, nil
}

func (a *App) GetSessions() ([]interface{}, error) {
	port := server.GetServerPort()
	_ = server.EnsureServerRunning(port)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/sessions", port))
	if err != nil {
		return []interface{}{}, nil
	}
	defer resp.Body.Close()

	var result []interface{}
	body, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(body, &result)
	return result, nil
}

type SplitPaneReq struct {
	SessionID    string               `json:"sessionId"`
	ParentPaneID string               `json:"parentPaneId"`
	Direction    server.SplitDirection `json:"direction"`
	Command      string               `json:"command"`
	Args         []string             `json:"args"`
	WorkDir      string               `json:"workDir"`
	Cols         int                  `json:"cols"`
	Rows         int                  `json:"rows"`
}

func (a *App) SplitPane(req SplitPaneReq) (map[string]interface{}, error) {
	port := server.GetServerPort()
	_ = server.EnsureServerRunning(port)

	bodyData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/api/sessions/split", port), "application/json", bytes.NewBuffer(bodyData))
	if err != nil {
		return nil, fmt.Errorf("failed to split pane: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (a *App) CloseSession(sessionID string) error {
	port := server.GetServerPort()
	_ = server.EnsureServerRunning(port)

	bodyData, err := json.Marshal(map[string]string{"sessionId": sessionID})
	if err != nil {
		return err
	}

	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/api/sessions/close-session", port), "application/json", bytes.NewBuffer(bodyData))
	if err != nil {
		return fmt.Errorf("failed to close session: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (a *App) ClosePane(sessionID, paneID string) error {
	port := server.GetServerPort()
	_ = server.EnsureServerRunning(port)

	bodyData, err := json.Marshal(map[string]string{
		"sessionId": sessionID,
		"paneId":    paneID,
	})
	if err != nil {
		return err
	}

	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/api/sessions/close-pane", port), "application/json", bytes.NewBuffer(bodyData))
	if err != nil {
		return fmt.Errorf("failed to close pane: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (a *App) SelectFile() (string, error) {
	file, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Executable File",
		Filters: []runtime.FileFilter{
			{DisplayName: "Executables (*.exe;*.cmd;*.bat;*.sh)", Pattern: "*.exe;*.cmd;*.bat;*.sh"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	return file, nil
}

func (a *App) SelectDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Working Directory",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// OpenWorkFolder opens the given directory in the OS file manager.
func (a *App) OpenWorkFolder(workDir string) error {
	return opendir.Open(workDir)
}

// GetSSHConfig loads the (DPAPI-encrypted) ssh settings and address book.
func (a *App) GetSSHConfig() (*ssh.Config, error) {
	return ssh.Load()
}

// SaveSSHConfig persists the ssh settings and address book.
func (a *App) SaveSSHConfig(cfg *ssh.Config) error {
	return ssh.Save(cfg)
}

// ExportSSH encrypts the current ssh config with the user-provided password
// and saves it through a file dialog for transfer to another machine.
func (a *App) ExportSSH(password string) error {
	cfg, err := ssh.Load()
	if err != nil {
		return err
	}
	data, err := ssh.Export(cfg, password)
	if err != nil {
		return err
	}

	file, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export SSH Addresses",
		DefaultFilename: "pmux-ssh-export.pmuxssh",
		Filters: []runtime.FileFilter{
			{DisplayName: "pmux SSH Export (*.pmuxssh)", Pattern: "*.pmuxssh"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return err
	}
	if file == "" {
		return nil // dialog cancelled
	}
	return os.WriteFile(file, data, 0600)
}

// ImportSSH reads a password-encrypted export file through a file dialog,
// decrypts it with the user-provided password, and replaces the current
// ssh config with the imported data.
func (a *App) ImportSSH(password string) error {
	file, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import SSH Addresses",
		Filters: []runtime.FileFilter{
			{DisplayName: "pmux SSH Export (*.pmuxssh)", Pattern: "*.pmuxssh"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return err
	}
	if file == "" {
		return nil // dialog cancelled
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	cfg, err := ssh.Import(data, password)
	if err != nil {
		return err
	}
	return ssh.Save(cfg)
}

func (a *App) QuitApp() {
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}
