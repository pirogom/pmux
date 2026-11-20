package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestMain isolates ~/.pmux for the whole package so tests never write to the
// developer's real config, token or ConPTY extraction directory.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "pmux_server_test_*")
	if err != nil {
		panic(err)
	}
	origHome, hadHome := os.LookupEnv("HOME")
	origProfile, hadProfile := os.LookupEnv("USERPROFILE")
	_ = os.Setenv("HOME", dir)
	_ = os.Setenv("USERPROFILE", dir)

	code := m.Run()

	if hadHome {
		_ = os.Setenv("HOME", origHome)
	} else {
		_ = os.Unsetenv("HOME")
	}
	if hadProfile {
		_ = os.Setenv("USERPROFILE", origProfile)
	} else {
		_ = os.Unsetenv("USERPROFILE")
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestAuthMiddleware(t *testing.T) {
	s := &Server{token: "secret-token"}
	h := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	tests := []struct {
		name   string
		target string
		header string
		want   int
	}{
		{"no token", "/api/sessions", "", http.StatusUnauthorized},
		{"wrong token", "/api/sessions", "Bearer nope", http.StatusUnauthorized},
		{"bearer token", "/api/sessions", "Bearer secret-token", http.StatusOK},
		{"raw header token", "/api/sessions", "secret-token", http.StatusOK},
		{"query token (websocket)", "/ws/pane/p1?token=secret-token", "", http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestAuthMiddlewareRejectsEmptyServerToken(t *testing.T) {
	s := &Server{token: ""}
	h := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("an empty configured token must never authenticate, got %d", rec.Code)
	}
}

func TestCORSOnlyAllowsWailsOrigins(t *testing.T) {
	h := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Allowed origin is echoed back.
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Origin", "http://wails.localhost")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://wails.localhost" {
		t.Fatalf("allowed origin not echoed, got %q", got)
	}

	// A random website must not receive CORS permission.
	req = httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("hostile origin must not be allowed, got %q", got)
	}

	// Preflight from a hostile origin is rejected outright.
	req = httptest.NewRequest(http.MethodOptions, "/api/sessions", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("hostile preflight should be 403, got %d", rec.Code)
	}
}

func TestWebSocketRejectsForeignOrigin(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: wsOriginPatterns})
		if err != nil {
			return // Accept already wrote an error response
		}
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}))
	defer ts.Close()

	dial := func(origin string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		header := http.Header{}
		if origin != "" {
			header.Set("Origin", origin)
		}
		conn, _, err := websocket.Dial(ctx, "ws"+ts.URL[4:], &websocket.DialOptions{HTTPHeader: header})
		if err == nil {
			_ = conn.Close(websocket.StatusNormalClosure, "")
		}
		return err
	}

	if err := dial("http://wails.localhost"); err != nil {
		t.Fatalf("Wails origin must be accepted, got %v", err)
	}
	if err := dial("https://evil.example"); err == nil {
		t.Fatalf("foreign origin must be rejected")
	}
}
