package git

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/skeema/knownhosts"
	"golang.org/x/crypto/ssh"
)

// resolveAuth determines the best available authentication method for the
// given remote, reusing credentials already present on the system:
//
//   - SSH remotes: system ssh-agent, then ~/.ssh private keys.
//   - HTTPS remotes: Windows Credential Manager, then ~/.git-credentials.
//
// It returns the auth method and a human readable description of what was
// used. A nil method means "anonymous".
func resolveAuth(remote *gogit.Remote) (transport.AuthMethod, string) {
	if remote == nil {
		return nil, ""
	}
	urls := remote.Config().URLs
	if len(urls) == 0 {
		return nil, ""
	}

	isSSH, user, host := parseRemoteURL(urls[0])
	if !isSSH {
		auth, desc := httpsAuth(host)
		return auth, desc
	}

	if user == "" {
		user = "git"
	}

	if auth, err := gitssh.NewSSHAgentAuth(user); err == nil {
		auth.HostKeyCallback = hostKeyCallback()
		return auth, "ssh-agent"
	}

	if auth, desc, err := sshKeyAuth(user); err == nil {
		return auth, desc
	}
	return nil, "ssh"
}

// sshKeyAuth tries to build a PublicKeys auth method from the user's
// ~/.ssh private keys. Passphrase-protected keys are skipped (use the
// ssh-agent for those).
func sshKeyAuth(user string) (transport.AuthMethod, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}
	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		keyPath := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(keyPath); err != nil {
			continue
		}
		pub, err := gitssh.NewPublicKeysFromFile(user, keyPath, "")
		if err != nil {
			// Encrypted keys can't be loaded without a passphrase.
			continue
		}
		pub.HostKeyCallback = hostKeyCallback()
		return pub, "ssh-key (" + name + ")", nil
	}
	return nil, "", fmt.Errorf("no usable ssh key found in %s", filepath.Join(home, ".ssh"))
}

// hostKeyCallback verifies against ~/.ssh/known_hosts when available, falling
// back to insecure host key checking otherwise.
func hostKeyCallback() ssh.HostKeyCallback {
	home, err := os.UserHomeDir()
	if err == nil {
		known := filepath.Join(home, ".ssh", "known_hosts")
		if fi, err := os.Stat(known); err == nil && fi.Size() > 0 {
			if cb, err := knownhosts.New(known); err == nil {
				return ssh.HostKeyCallback(cb)
			}
		}
	}
	return ssh.InsecureIgnoreHostKey()
}

// httpsAuth looks up stored HTTPS credentials for host, first in the Windows
// Credential Manager then in the ~/.git-credentials file.
func httpsAuth(host string) (transport.AuthMethod, string) {
	if auth, desc, ok := wincredAuth(host); ok {
		return auth, desc
	}
	if auth, desc, ok := gitCredentialsAuth(host); ok {
		return auth, desc
	}
	return nil, "none"
}

// gitCredentialsAuth parses the plain-text ~/.git-credentials file (the
// credential-store format) looking for credentials matching host.
func gitCredentialsAuth(host string) (transport.AuthMethod, string, bool) {
	paths := []string{}
	if env := os.Getenv("GIT_CREDENTIALS_FILE"); env != "" {
		paths = append(paths, env)
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".git-credentials"))
	}

	host = strings.ToLower(host)
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			u, err := url.Parse(line)
			if err != nil || u.User == nil {
				continue
			}
			if strings.ToLower(u.Hostname()) != host {
				continue
			}
			pass, _ := u.User.Password()
			return &githttp.BasicAuth{Username: u.User.Username(), Password: pass}, "git-credentials", true
		}
	}
	return nil, "", false
}

// parseRemoteURL classifies a remote URL. Returns isSSH, the user (may be
// empty) and the host (empty for local paths).
func parseRemoteURL(raw string) (isSSH bool, user, host string) {
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "ssh://"),
		strings.HasPrefix(lower, "git+ssh://"),
		strings.HasPrefix(lower, "ssh+git://"):
		if u, err := url.Parse(raw); err == nil {
			user = ""
			if u.User != nil {
				user = u.User.Username()
			}
			return true, user, u.Hostname()
		}
		return true, "", ""
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		u, err := url.Parse(raw)
		if err != nil {
			return false, "", ""
		}
		if u.User != nil {
			user = u.User.Username()
		}
		return false, user, u.Hostname()
	case strings.Contains(raw, "://"):
		return false, "", ""
	default:
		// scp-like syntax: [user@]host:path
		if strings.Contains(raw, ":") {
			before := raw[:strings.Index(raw, ":")]
			if at := strings.LastIndex(before, "@"); at != -1 {
				return true, before[:at], before[at+1:]
			}
		}
		return false, "", "" // local path
	}
}
