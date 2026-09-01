package git

import (
	"bytes"
	"io"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// coreAutocrlf returns the effective core.autocrlf value. go-git's Worktree
// does not implement core.autocrlf at all, so line-ending normalization has to
// be handled by this client. Precedence matches git: local config, then global
// (~/.gitconfig), then system (e.g. the value the Git for Windows installer
// sets).
func coreAutocrlf(repo *gogit.Repository) string {
	if cfg, err := repo.Config(); err == nil {
		if v := cfg.Raw.Section("core").Option("autocrlf"); v != "" {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	for _, scope := range []gitconfig.Scope{gitconfig.GlobalScope, gitconfig.SystemScope} {
		if c, err := gitconfig.LoadConfig(scope); err == nil {
			if v := c.Raw.Section("core").Option("autocrlf"); v != "" {
				return strings.ToLower(strings.TrimSpace(v))
			}
		}
	}
	return ""
}

// isAutocrlfEnabled reports whether the autocrlf clean filter (CRLF -> LF) must
// be applied before hashing or storing file content.
func isAutocrlfEnabled(repo *gogit.Repository) bool {
	switch coreAutocrlf(repo) {
	case "true", "input", "yes", "on", "1":
		return true
	default:
		return false
	}
}

// isTextContent applies git's binary heuristic: a file is treated as text
// unless it contains a NUL byte. This matches git's text=auto behavior, the
// default when core.autocrlf is enabled without a .gitattributes override.
func isTextContent(data []byte) bool {
	return !bytes.ContainsRune(data, 0)
}

// cleanLineEndings applies the autocrlf clean filter: CRLF pairs become LF.
// Lone CR characters are kept, matching git's conversion.
func cleanLineEndings(data []byte) []byte {
	if !bytes.ContainsRune(data, '\r') {
		return data
	}
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			continue
		}
		out = append(out, data[i])
	}
	return out
}

// worktreeBlobData returns the given worktree file's content with the autocrlf
// clean filter applied (exactly what official git hashes on add/status), plus
// the blob hash for that filtered content.
func worktreeBlobData(bundle *repoBundle, path string) ([]byte, plumbing.Hash, bool) {
	f, err := bundle.worktree.Filesystem.Open(path)
	if err != nil {
		return nil, plumbing.ZeroHash, false
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, plumbing.ZeroHash, false
	}

	if isAutocrlfEnabled(bundle.repo) && isTextContent(data) {
		data = cleanLineEndings(data)
	}

	h := plumbing.NewHasher(plumbing.BlobObject, int64(len(data)))
	if _, err := h.Write(data); err != nil {
		return nil, plumbing.ZeroHash, false
	}
	return data, h.Sum(), true
}
