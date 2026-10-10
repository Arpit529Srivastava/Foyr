package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/config"
	"github.com/Arpit529Srivastava/Foyr/internal/github"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
)

// syncBuffer is a log sink that is safe for the background goroutine.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func waitFor(t *testing.T, logs *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logs.String(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("log never contained %q:\n%s", want, logs.String())
}

func setup(t *testing.T, githubStatus int) (http.Handler, *syncBuffer, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/access_tokens"):
			w.WriteHeader(githubStatus)
			fmt.Fprint(w, `{"token":"ghs_realsecretvalue","expires_at":"2026-10-10T10:00:00Z"}`)
		case strings.Contains(r.URL.Path, "/commits/"):
			fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.Close)

	secret := []byte("a-long-random-webhook-secret")
	cfg := &config.Config{GitHubAppID: 123456, GitHubPrivateKey: key, GitHubWebhookSecret: secret, GitHubAPIURL: fake.URL}
	logs := &syncBuffer{}
	return newServer(cfg, obs.NewLogger(logs)), logs, secret
}

func post(h http.Handler, event string, body []byte, secret []byte) int {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-GitHub-Delivery", "d-1")
	req.Header.Set("X-Hub-Signature-256", github.Sign(secret, body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestPushEndToEnd(t *testing.T) {
	h, logs, secret := setup(t, http.StatusCreated)
	body, _ := os.ReadFile("../../internal/github/testdata/push.json")
	if code := post(h, "push", body, secret); code != http.StatusAccepted {
		t.Fatalf("status = %d", code)
	}
	waitFor(t, logs, `"msg":"push received"`)
	waitFor(t, logs, `"msg":"repo access ok"`)
	if strings.Contains(logs.String(), "realsecret") {
		t.Fatal("installation token leaked into logs")
	}
	if !strings.Contains(logs.String(), `"request_id"`) {
		t.Fatal("logs missing request id")
	}
}

func TestPushTokenFailureIsLogged(t *testing.T) {
	h, logs, secret := setup(t, http.StatusUnauthorized)
	body, _ := os.ReadFile("../../internal/github/testdata/push.json")
	post(h, "push", body, secret)
	waitFor(t, logs, `"msg":"installation token failed"`)
}

func TestDeletedBranchIsSkipped(t *testing.T) {
	h, logs, secret := setup(t, http.StatusCreated)
	body, _ := os.ReadFile("../../internal/github/testdata/push_delete.json")
	post(h, "push", body, secret)
	waitFor(t, logs, `"reason":"branch deleted"`)
	time.Sleep(100 * time.Millisecond)
	if strings.Contains(logs.String(), "repo access") {
		t.Fatal("deleted branch should not trigger a repo check")
	}
}

func TestInstallationIsLogged(t *testing.T) {
	h, logs, secret := setup(t, http.StatusCreated)
	body, _ := os.ReadFile("../../internal/github/testdata/installation.json")
	post(h, "installation", body, secret)
	waitFor(t, logs, `"repos":["octo-dev/foyr-hello"]`)
}

func TestRoutes(t *testing.T) {
	h, _, _ := setup(t, http.StatusCreated)
	for _, tt := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/webhooks/github", http.StatusMethodNotAllowed},
		{http.MethodGet, "/nope", http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != tt.want {
			body, _ := io.ReadAll(rec.Body)
			t.Errorf("%s %s = %d, want %d (%s)", tt.method, tt.path, rec.Code, tt.want, body)
		}
	}
}
