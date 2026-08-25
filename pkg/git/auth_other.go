//go:build !windows

package git

import "github.com/go-git/go-git/v5/plumbing/transport"

// wincredAuth is a no-op on non-Windows platforms (no Windows Credential
// Manager); ~/.git-credentials is handled in auth.go.
func wincredAuth(host string) (transport.AuthMethod, string, bool) {
	return nil, "", false
}
