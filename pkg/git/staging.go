package git

import (
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"
)

// Stage adds the given paths to the index (git add).
func Stage(workDir string, paths []string) GitOpResult {
	bundle, err := openRepo(workDir)
	if err != nil {
		return opErr(err)
	}
	staged := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if err := bundle.worktree.AddWithOptions(&gogit.AddOptions{Path: p}); err != nil {
			return opErr(fmt.Errorf("failed to stage %q: %w", p, err))
		}
		staged = append(staged, p)
	}
	if len(staged) == 0 {
		return opOK("Nothing to stage.")
	}
	return opOK("Staged " + strings.Join(staged, ", "))
}

// Unstage removes the given paths from the index (git reset -- <paths>).
func Unstage(workDir string, paths []string) GitOpResult {
	bundle, err := openRepo(workDir)
	if err != nil {
		return opErr(err)
	}
	unstaged := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if err := bundle.worktree.Reset(&gogit.ResetOptions{Files: []string{p}}); err != nil {
			return opErr(fmt.Errorf("failed to unstage %q: %w", p, err))
		}
		unstaged = append(unstaged, p)
	}
	if len(unstaged) == 0 {
		return opOK("Nothing to unstage.")
	}
	return opOK("Unstaged " + strings.Join(unstaged, ", "))
}

// StageAll stages every change in the working tree (git add -A).
func StageAll(workDir string) GitOpResult {
	bundle, err := openRepo(workDir)
	if err != nil {
		return opErr(err)
	}
	if err := bundle.worktree.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return opErr(fmt.Errorf("failed to stage all: %w", err))
	}
	return opOK("All changes staged.")
}

func opErr(err error) GitOpResult {
	return GitOpResult{Success: false, Error: err.Error()}
}

func opOK(output string) GitOpResult {
	return GitOpResult{Success: true, Output: output}
}
