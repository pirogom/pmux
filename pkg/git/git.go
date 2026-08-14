package git

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
)

type GitChange struct {
	Status string `json:"status"` // "M", "A", "D", "??" 등
	Path   string `json:"path"`
}

type GitStatusResult struct {
	IsGitRepo bool        `json:"isGitRepo"`
	Branch    string      `json:"branch"`
	Changes   []GitChange `json:"changes"`
	Error     string      `json:"error,omitempty"`
}

func GetStatus(workDir string) GitStatusResult {
	if workDir == "" {
		return GitStatusResult{IsGitRepo: false}
	}

	// 1. Check if git command exists
	_, err := exec.LookPath("git")
	if err != nil {
		return GitStatusResult{IsGitRepo: false, Error: "git command not found"}
	}

	// 2. Check branch
	cmdBranch := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	prepareCommand(cmdBranch)
	cmdBranch.Dir = workDir
	var outBranch bytes.Buffer
	cmdBranch.Stdout = &outBranch
	if err := cmdBranch.Run(); err != nil {
		return GitStatusResult{IsGitRepo: false}
	}
	branch := strings.TrimSpace(outBranch.String())

	// 3. git status --porcelain
	cmdStatus := exec.Command("git", "status", "--porcelain")
	prepareCommand(cmdStatus)
	cmdStatus.Dir = workDir
	var outStatus bytes.Buffer
	cmdStatus.Stdout = &outStatus
	if err := cmdStatus.Run(); err != nil {
		return GitStatusResult{IsGitRepo: true, Branch: branch}
	}

	var changes []GitChange
	lines := strings.Split(outStatus.String(), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		status := strings.TrimSpace(line[:2])
		filePath := strings.TrimSpace(line[3:])
		changes = append(changes, GitChange{
			Status: status,
			Path:   filePath,
		})
	}

	return GitStatusResult{
		IsGitRepo: true,
		Branch:    branch,
		Changes:   changes,
	}
}

func Push(workDir string) string {
	cmd := exec.Command("git", "push")
	prepareCommand(cmd)
	cmd.Dir = workDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return "Error: " + err.Error() + "\n" + out.String()
	}
	return out.String()
}

func Pull(workDir string) string {
	cmd := exec.Command("git", "pull")
	prepareCommand(cmd)
	cmd.Dir = workDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return "Error: " + err.Error() + "\n" + out.String()
	}
	return out.String()
}

func GetRepoRoot(workDir string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	prepareCommand(cmd)
	cmd.Dir = workDir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return workDir
	}
	root := strings.TrimSpace(out.String())
	return filepath.Clean(root)
}
