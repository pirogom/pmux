package git

import (
	"fmt"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// Stage adds the given paths to the index (git add).
func Stage(workDir string, paths []string) GitOpResult {
	bundle, err := openRepo(workDir)
	if err != nil {
		return opErr(err)
	}
	var prevModes map[string]filemode.FileMode
	if isFileModeIgnored(bundle.repo) {
		prevModes = getExecutableModes(bundle)
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
	fixupCleanFilter(bundle, staged)
	if isFileModeIgnored(bundle.repo) && len(prevModes) > 0 {
		restoreExecutableModes(bundle, prevModes)
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
	var prevModes map[string]filemode.FileMode
	if isFileModeIgnored(bundle.repo) {
		prevModes = getExecutableModes(bundle)
	}
	if err := bundle.worktree.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return opErr(fmt.Errorf("failed to stage all: %w", err))
	}
	fixupCleanFilter(bundle, nil)
	if isFileModeIgnored(bundle.repo) && len(prevModes) > 0 {
		restoreExecutableModes(bundle, prevModes)
	}
	return opOK("All changes staged.")
}

// fixupCleanFilter rewrites staged index entries whose on-disk content has CRLF
// line endings so the stored blob matches the autocrlf clean filter, exactly as
// official git does on `git add`. go-git's Add() copies raw bytes, so without
// this step commits would diverge from git's line-ending normalization. If
// paths is nil, every index entry is checked.
func fixupCleanFilter(bundle *repoBundle, paths []string) {
	if !isAutocrlfEnabled(bundle.repo) {
		return
	}
	idx, err := bundle.repo.Storer.Index()
	if err != nil {
		return
	}

	var entries []*index.Entry
	if paths == nil {
		entries = idx.Entries
	} else {
		for _, p := range paths {
			if e, err := idx.Entry(filepath.ToSlash(filepath.Clean(p))); err == nil && e != nil {
				entries = append(entries, e)
			}
		}
	}

	modified := false
	for _, e := range entries {
		if e.Mode == filemode.Symlink {
			continue
		}
		data, hash, ok := worktreeBlobData(bundle, e.Name)
		if !ok || hash == e.Hash {
			continue
		}
		obj := bundle.repo.Storer.NewEncodedObject()
		obj.SetType(plumbing.BlobObject)
		obj.SetSize(int64(len(data)))
		w, err := obj.Writer()
		if err != nil {
			continue
		}
		if _, err := w.Write(data); err != nil {
			_ = w.Close()
			continue
		}
		if err := w.Close(); err != nil {
			continue
		}
		stored, err := bundle.repo.Storer.SetEncodedObject(obj)
		if err != nil {
			continue
		}
		e.Hash = stored
		modified = true
	}
	if modified {
		_ = bundle.repo.Storer.SetIndex(idx)
	}
}

func getExecutableModes(bundle *repoBundle) map[string]filemode.FileMode {
	modes := make(map[string]filemode.FileMode)
	idx, err := bundle.repo.Storer.Index()
	if err != nil {
		return modes
	}
	for _, entry := range idx.Entries {
		if entry.Mode == filemode.Executable {
			modes[entry.Name] = filemode.Executable
		}
	}
	return modes
}

func restoreExecutableModes(bundle *repoBundle, prevModes map[string]filemode.FileMode) {
	if len(prevModes) == 0 {
		return
	}
	idx, err := bundle.repo.Storer.Index()
	if err != nil {
		return
	}
	modified := false
	for _, entry := range idx.Entries {
		if mode, ok := prevModes[entry.Name]; ok && mode == filemode.Executable && entry.Mode != filemode.Executable {
			entry.Mode = filemode.Executable
			modified = true
		}
	}
	if modified {
		_ = bundle.repo.Storer.SetIndex(idx)
	}
}

func opErr(err error) GitOpResult {
	return GitOpResult{Success: false, Error: err.Error()}
}

func opOK(output string) GitOpResult {
	return GitOpResult{Success: true, Output: output}
}
