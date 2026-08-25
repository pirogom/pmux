package git

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitdiff "github.com/go-git/go-git/v5/utils/diff"
)

// Commit creates a commit with the given message using the user identity from
// the repository (or global) git config.
func Commit(workDir, message string) GitOpResult {
	if strings.TrimSpace(message) == "" {
		return opErr(errors.New("commit message is empty"))
	}
	bundle, err := openRepo(workDir)
	if err != nil {
		return opErr(err)
	}

	author, err := commitAuthor(bundle)
	if err != nil {
		return opErr(err)
	}

	hash, err := bundle.worktree.Commit(strings.TrimSpace(message), &gogit.CommitOptions{Author: author})
	if err != nil {
		return opErr(fmt.Errorf("commit failed: %w", err))
	}
	return opOK(fmt.Sprintf("[%s] %s", currentBranch(bundle.repo), subjectLine(strings.TrimSpace(message))) +
		"\n" + shortHash(hash.String()))
}

// commitAuthor resolves the user.name / user.email identity from the local
// repository config, falling back to the global (~/.gitconfig) config.
func commitAuthor(b *repoBundle) (*object.Signature, error) {
	sig := &object.Signature{When: time.Now()}

	// 1. Local repository config.
	if cfg, err := b.repo.Config(); err == nil {
		sig.Name = cfg.User.Name
		sig.Email = cfg.User.Email
	}

	// 2. Global config.
	if sig.Name == "" || sig.Email == "" {
		if gc, err := gitconfig.LoadConfig(gitconfig.GlobalScope); err == nil {
			if sig.Name == "" {
				sig.Name = gc.User.Name
			}
			if sig.Email == "" {
				sig.Email = gc.User.Email
			}
		}
	}

	if sig.Name == "" || sig.Email == "" {
		return nil, errors.New("commit requires user.name and user.email (set with `git config --global user.name \"...\"` and `git config --global user.email \"...\"`)")
	}
	return sig, nil
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// GitCommitDetail is the metadata plus a textual diff of a single commit.
type GitCommitDetail struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"shortHash"`
	Author    string `json:"author"`
	Email     string `json:"email"`
	Date      string `json:"date"`
	Subject   string `json:"subject"`
	Message   string `json:"message"`
	Parent    string `json:"parent,omitempty"`
	Diff      string `json:"diff"`
	Error     string `json:"error,omitempty"`
}

// GetCommitDetail returns a commit's metadata and its diff against the first
// parent (the empty tree for root commits).
func GetCommitDetail(workDir, hash string) GitCommitDetail {
	res := GitCommitDetail{Hash: hash, ShortHash: shortHash(hash)}
	bundle, err := openRepo(workDir)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	commit, err := bundle.repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		res.Error = fmt.Sprintf("commit not found: %s", hash)
		return res
	}

	res.Hash = commit.Hash.String()
	res.ShortHash = shortHash(res.Hash)
	res.Author = commit.Author.Name
	res.Email = commit.Author.Email
	res.Date = commit.Author.When.Format("2006-01-02 15:04")
	res.Message = strings.TrimRight(commit.Message, "\n")
	res.Subject = subjectLine(res.Message)

	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		if parent, err := commit.Parent(0); err == nil {
			res.Parent = parent.Hash.String()
			parentTree, _ = parent.Tree()
		}
	}

	commitTree, err := commit.Tree()
	if err != nil {
		res.Error = fmt.Sprintf("failed to read tree: %v", err)
		return res
	}

	changes, err := object.DiffTree(parentTree, commitTree)
	if err != nil {
		res.Error = fmt.Sprintf("failed to diff: %v", err)
		return res
	}

	var b strings.Builder
	for _, ch := range changes {
		fromPath, toPath := "", ""
		var oldContent, newContent string
		if !ch.From.TreeEntry.Hash.IsZero() {
			fromPath = ch.From.Name
			oldContent = blobContents(bundle.repo, ch.From.TreeEntry.Hash)
		}
		if !ch.To.TreeEntry.Hash.IsZero() {
			toPath = ch.To.Name
			newContent = blobContents(bundle.repo, ch.To.TreeEntry.Hash)
		}
		b.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", fromPath, toPath))
		if isBinary(oldContent) || isBinary(newContent) {
			b.WriteString("Binary files differ\n")
			continue
		}
		oldContent = strings.ReplaceAll(oldContent, "\r\n", "\n")
		newContent = strings.ReplaceAll(newContent, "\r\n", "\n")
		b.WriteString(renderDiff(gogitdiff.Do(oldContent, newContent)))
	}
	res.Diff = b.String()
	return res
}

func blobContents(repo *gogit.Repository, h plumbing.Hash) string {
	blob, err := repo.BlobObject(h)
	if err != nil {
		return ""
	}
	reader, err := blob.Reader()
	if err != nil {
		return ""
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return ""
	}
	return string(data)
}
