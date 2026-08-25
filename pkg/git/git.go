// Package git implements a pure-Go git client backed by go-git (no cgo, no
// dependency on the system git executable).
package git

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// ErrNotGitRepo indicates the given directory is not inside a git work tree.
var ErrNotGitRepo = errors.New("not a git repository")

// GitChange represents a single changed file in the working tree.
type GitChange struct {
	Status   string `json:"status"` // e.g. "M", "A", "D", "??", "MM"
	Staged   bool   `json:"staged"`
	Unstaged bool   `json:"unstaged"`
	Path     string `json:"path"`
}

// GitStatusResult is the result of a repository status inspection.
type GitStatusResult struct {
	IsGitRepo bool        `json:"isGitRepo"`
	Branch    string      `json:"branch"`
	Root      string      `json:"root,omitempty"`
	Changes   []GitChange `json:"changes"`
	Error     string      `json:"error,omitempty"`
}

// GitCommit is a single commit in the repository log.
type GitCommit struct {
	Hash      string   `json:"hash"`
	ShortHash string   `json:"shortHash"`
	Author    string   `json:"author"`
	Email     string   `json:"email"`
	Date      string   `json:"date"`
	Message   string   `json:"message"`
	Subject   string   `json:"subject"`
	Refs      []string `json:"refs,omitempty"`
	IsHead    bool     `json:"isHead"`
}

// GitBranch is a local branch.
type GitBranch struct {
	Name     string `json:"name"`
	Current  bool   `json:"current"`
	Upstream string `json:"upstream,omitempty"`
}

// GitRemote is a configured remote.
type GitRemote struct {
	Name string   `json:"name"`
	URLs []string `json:"urls"`
}

// GitDiffResult contains the staged and unstaged diff for a single path.
type GitDiffResult struct {
	Path     string `json:"path"`
	Staged   string `json:"staged,omitempty"`
	Unstaged string `json:"unstaged,omitempty"`
	Binary   bool   `json:"binary"`
	Error    string `json:"error,omitempty"`
}

// GitOpResult is the generic result of a git operation (stage, commit, push...).
type GitOpResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
	Auth    string `json:"auth,omitempty"`
}

// repoBundle bundles an opened repository with its worktree and root path.
type repoBundle struct {
	repo     *gogit.Repository
	worktree *gogit.Worktree
	root     string
}

// openRepo opens the repository containing workDir, searching upwards, and
// returns the bundle with the worktree root.
func openRepo(workDir string) (*repoBundle, error) {
	if strings.TrimSpace(workDir) == "" {
		return nil, ErrNotGitRepo
	}
	repo, err := gogit.PlainOpenWithOptions(workDir, &gogit.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		if errors.Is(err, gogit.ErrRepositoryNotExists) {
			return nil, ErrNotGitRepo
		}
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to open worktree: %w", err)
	}
	root := filepath.Clean(wt.Filesystem.Root())
	return &repoBundle{repo: repo, worktree: wt, root: root}, nil
}

// currentBranch resolves the branch HEAD points to, handling detached HEAD and
// unborn branches (empty repositories).
func currentBranch(repo *gogit.Repository) string {
	head, err := repo.Head()
	if err == nil {
		if head.Name().IsBranch() {
			return head.Name().Short()
		}
		hash := head.Hash().String()
		if len(hash) > 7 {
			hash = hash[:7]
		}
		return hash + " (detached)"
	}
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		if sym, serr := repo.Reference(plumbing.HEAD, false); serr == nil && sym.Type() == plumbing.SymbolicReference {
			return sym.Target().Short() + " (no commits)"
		}
		return "(no commits)"
	}
	return ""
}
