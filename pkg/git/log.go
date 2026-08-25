package git

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitdiff "github.com/go-git/go-git/v5/utils/diff"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// GetLog returns up to limit commits reachable from HEAD, most recent first.
func GetLog(workDir string, limit int) ([]GitCommit, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	bundle, err := openRepo(workDir)
	if err != nil {
		return nil, err
	}

	head, err := bundle.repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return []GitCommit{}, nil
		}
		return nil, err
	}

	iter, err := bundle.repo.Log(&gogit.LogOptions{
		From:  head.Hash(),
		Order: gogit.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	refMap := branchRefMap(bundle)

	commits := make([]GitCommit, 0, limit)
	for i := 0; i < limit; i++ {
		c, err := iter.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return commits, err
		}
		hash := c.Hash.String()
		short := hash
		if len(short) > 7 {
			short = short[:7]
		}
		msg := strings.TrimRight(c.Message, "\n")
		commits = append(commits, GitCommit{
			Hash:      hash,
			ShortHash: short,
			Author:    c.Author.Name,
			Email:     c.Author.Email,
			Date:      c.Author.When.Format("2006-01-02 15:04"),
			Message:   msg,
			Subject:   subjectLine(msg),
			Refs:      refMap[hash],
			IsHead:    i == 0,
		})
	}
	return commits, nil
}

// GetDiff returns the staged and unstaged diff for a single path.
func GetDiff(workDir, path string) GitDiffResult {
	res := GitDiffResult{Path: path}
	bundle, err := openRepo(workDir)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	headContent, headExists, err := headFileContents(bundle, path)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	indexContent, indexExists, err := indexFileContents(bundle, path)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	workContent, workExists, err := workFileContents(bundle, path)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	res.Binary = isBinary(headContent) || isBinary(indexContent) || isBinary(workContent)
	if res.Binary {
		return res
	}

	// Normalize line endings so CRLF/LF differences do not make the whole
	// file appear changed.
	headContent = strings.ReplaceAll(headContent, "\r\n", "\n")
	indexContent = strings.ReplaceAll(indexContent, "\r\n", "\n")
	workContent = strings.ReplaceAll(workContent, "\r\n", "\n")

	if headExists || indexExists {
		res.Staged = renderDiff(gogitdiff.Do(headContent, indexContent))
	}
	if indexExists || workExists {
		res.Unstaged = renderDiff(gogitdiff.Do(indexContent, workContent))
	}
	return res
}

// branchRefMap maps commit hashes to the names of the refs pointing at them.
func branchRefMap(b *repoBundle) map[string][]string {
	refMap := map[string][]string{}
	refIter, err := b.repo.References()
	if err != nil {
		return refMap
	}
	_ = refIter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.SymbolicReference {
			return nil
		}
		if ref.Name().IsBranch() || ref.Name().IsRemote() || ref.Name().IsTag() {
			name := ref.Name().Short()
			if ref.Name().IsTag() {
				name = "tag: " + name
			}
			refMap[ref.Hash().String()] = append(refMap[ref.Hash().String()], name)
		}
		return nil
	})
	return refMap
}

func headFileContents(b *repoBundle, path string) (string, bool, error) {
	head, err := b.repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	commit, err := b.repo.CommitObject(head.Hash())
	if err != nil {
		return "", false, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return "", false, err
	}
	f, err := tree.File(path)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	content, err := f.Contents()
	return content, true, err
}

func indexFileContents(b *repoBundle, path string) (string, bool, error) {
	idx, err := b.repo.Storer.Index()
	if err != nil {
		return "", false, err
	}
	e, err := idx.Entry(path)
	if err != nil {
		if errors.Is(err, index.ErrEntryNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	blob, err := b.repo.BlobObject(e.Hash)
	if err != nil {
		return "", false, err
	}
	reader, err := blob.Reader()
	if err != nil {
		return "", false, err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

func workFileContents(b *repoBundle, path string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(b.root, filepath.FromSlash(path)))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}

func subjectLine(msg string) string {
	msg = strings.TrimSpace(msg)
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		return msg[:idx]
	}
	return msg
}

func isBinary(content string) bool {
	return strings.ContainsRune(content, '\x00')
}

// renderDiff renders line diffs (git-style -/+ prefixes) into text. It returns
// an empty string when there is nothing to show (identical contents).
func renderDiff(diffs []diffmatchpatch.Diff) string {
	hasChange := false
	for _, d := range diffs {
		if d.Type != diffmatchpatch.DiffEqual {
			hasChange = true
			break
		}
	}
	if !hasChange {
		return ""
	}

	var b strings.Builder
	for _, d := range diffs {
		prefix := " "
		switch d.Type {
		case diffmatchpatch.DiffDelete:
			prefix = "-"
		case diffmatchpatch.DiffInsert:
			prefix = "+"
		}
		lines := strings.Split(d.Text, "\n")
		for i, ln := range lines {
			if i == len(lines)-1 && ln == "" {
				continue
			}
			b.WriteString(prefix)
			b.WriteString(ln)
			b.WriteString("\n")
		}
	}
	return b.String()
}
