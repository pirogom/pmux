package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"pmux/pkg/config"
)

// The background daemon listens on 127.0.0.1, which any local process (and any
// web page the user visits, thanks to browser fetch) can reach. Every request
// therefore has to present the shared token stored in ~/.pmux/token (mode
// 0600), which only the same OS user can read.
var (
	tokenOnce sync.Once
	tokenVal  string
	tokenErr  error
)

const tokenFileName = "token"

func tokenFilePath() (string, error) {
	dir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, tokenFileName), nil
}

// GetToken returns the shared daemon token, creating it on first use. The
// create is O_EXCL so two processes racing at startup converge on one value.
// PMUX_TOKEN (set by the GUI when it spawns the daemon) takes precedence so the
// two processes always agree even if the token file cannot be written.
func GetToken() (string, error) {
	tokenOnce.Do(func() {
		if env := strings.TrimSpace(os.Getenv("PMUX_TOKEN")); env != "" {
			tokenVal = env
			if path, err := tokenFilePath(); err == nil {
				if data, _ := os.ReadFile(path); strings.TrimSpace(string(data)) != env {
					_ = os.WriteFile(path, []byte(env+"\n"), 0600)
				}
			}
			return
		}

		path, err := tokenFilePath()
		if err != nil {
			tokenErr = err
			return
		}

		if data, err := os.ReadFile(path); err == nil {
			if t := strings.TrimSpace(string(data)); t != "" {
				tokenVal = t
				return
			}
		}

		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			tokenErr = err
			return
		}
		candidate := hex.EncodeToString(buf)

		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, werr := f.WriteString(candidate + "\n")
			_ = f.Close()
			if werr != nil {
				tokenErr = werr
				return
			}
			tokenVal = candidate
			return
		}

		// Lost the creation race (or a stale empty file exists): read the
		// winner's token.
		if data, rerr := os.ReadFile(path); rerr == nil {
			if t := strings.TrimSpace(string(data)); t != "" {
				tokenVal = t
				return
			}
		}
		tokenErr = err
	})

	return tokenVal, tokenErr
}

// MustToken returns the token or an empty string when it cannot be resolved.
// Callers that require auth must treat an empty token as a fatal error.
func MustToken() string {
	t, err := GetToken()
	if err != nil {
		log.Printf("[pmux auth] failed to resolve token: %v", err)
	}
	return t
}

func requestToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if strings.HasPrefix(h, "Bearer ") {
			return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		}
		return strings.TrimSpace(h)
	}
	// Browsers cannot set headers on WebSocket handshakes, so allow the token
	// as a query parameter for /ws/* endpoints.
	return r.URL.Query().Get("token")
}

func tokenMatches(got, want string) bool {
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// authMiddleware rejects requests that do not present the daemon token.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !tokenMatches(requestToken(r), s.token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allowedOrigins lists the origins the Wails webview (and its dev server) use.
// Anything else is treated as a hostile page and gets no CORS permission.
var allowedOrigins = map[string]bool{
	"wails://wails":           true,
	"wails://wails.localhost": true,
	"http://wails.localhost":  true,
	"https://wails.localhost": true,
	"http://localhost:34115":  true,
	"http://127.0.0.1:34115":  true,
}

// isAllowedOrigin additionally accepts any loopback origin, so a Wails dev
// server that picked a different port still works. A hostile website can never
// have a loopback origin.
func isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	if allowedOrigins[origin] {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "http", "https", "wails":
	default:
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "wails.localhost"
}

// wsOriginPatterns mirrors isAllowedOrigin for the WebSocket Origin check
// (coder/websocket matches these with path.Match against the Origin URL host).
var wsOriginPatterns = []string{
	"wails",
	"wails.localhost",
	"localhost",
	"localhost:*",
	"127.0.0.1",
	"127.0.0.1:*",
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			if origin != "" && !isAllowedOrigin(origin) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var errNoToken = errors.New("pmux: daemon token unavailable")
