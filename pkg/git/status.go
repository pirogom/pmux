package git

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
)

// GetStatus inspects the repository containing workDir and returns the current
// branch plus all changed files (staged and/or unstaged).
func GetStatus(workDir string) GitStatusResult {
	bundle, err := openRepo(workDir)
	if err != nil {
		if errors.Is(err, ErrNotGitRepo) {
			return GitStatusResult{IsGitRepo: false}
		}
		return GitStatusResult{IsGitRepo: false, Error: err.Error()}
	}

	branch := currentBranch(bundle.repo)

	st, err := bundle.worktree.Status()
	if err != nil {
		return GitStatusResult{IsGitRepo: true, Branch: branch, Root: bundle.root, Error: err.Error()}
	}

	changes := make([]GitChange, 0, len(st))
	for path, fs := range st {
		status := strings.TrimSpace(fmt.Sprintf("%c%c", fs.Staging, fs.Worktree))
		if fs.Staging == gogit.Renamed && fs.Extra != "" {
			status = "R"
		}
		changes = append(changes, GitChange{
			Status:   status,
			Staged:   fs.Staging != gogit.Untracked && fs.Staging != gogit.Unmodified,
			Unstaged: fs.Worktree != gogit.Unmodified,
			Path:     path,
		})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })

	return GitStatusResult{
		IsGitRepo: true,
		Branch:    branch,
		Root:      bundle.root,
		Changes:   changes,
	}
}

// GetRepoRoot returns the root directory of the repository containing workDir,
// or workDir itself if no repository was found.
func GetRepoRoot(workDir string) string {
	bundle, err := openRepo(workDir)
	if err != nil {
		return workDir
	}
	return bundle.root
}
