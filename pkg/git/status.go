package git

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/index"
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

	ignoreMode := isFileModeIgnored(bundle.repo)
	var idx *index.Index
	if ignoreMode {
		idx, _ = bundle.repo.Storer.Index()
	}

	changes := make([]GitChange, 0, len(st))
	for path, fs := range st {
		if ignoreMode && idx != nil {
			if fs.Worktree == gogit.Modified {
				if entry, err := idx.Entry(path); err == nil && entry != nil {
					if isSameContent(bundle, path, entry.Hash) {
						fs.Worktree = gogit.Unmodified
					}
				}
			}
			if fs.Staging == gogit.Modified {
				if headHash, ok := headFileHash(bundle, path); ok {
					if entry, err := idx.Entry(path); err == nil && entry != nil && headHash == entry.Hash {
						fs.Staging = gogit.Unmodified
					}
				}
			}
		}

		if fs.Staging == gogit.Unmodified && fs.Worktree == gogit.Unmodified {
			continue
		}

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
