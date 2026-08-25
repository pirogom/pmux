//go:build windows

package git

import (
	"errors"

	"github.com/danieljoos/wincred"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// wincredAuth looks up HTTPS credentials stored in the Windows Credential
// Manager by git.exe / Git Credential Manager (target "git:https://host").
func wincredAuth(host string) (transport.AuthMethod, string, bool) {
	targets := []string{
		"git:https://" + host,
		"git:http://" + host,
		"git:" + host,
	}
	for _, target := range targets {
		cred, err := wincred.GetGenericCredential(target)
		if err != nil {
			if errors.Is(err, wincred.ErrElementNotFound) {
				continue
			}
			continue
		}
		if cred == nil || cred.UserName == "" || len(cred.CredentialBlob) == 0 {
			continue
		}
		return &githttp.BasicAuth{
			Username: cred.UserName,
			Password: string(cred.CredentialBlob),
		}, "credential-manager", true
	}
	return nil, "", false
}
