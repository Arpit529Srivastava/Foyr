package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	keyOnce sync.Once
	testKey *rsa.PrivateKey
)

func key(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testKey = k
	})
	return testKey
}

var fixedNow = time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

func TestJWT(t *testing.T) {
	app := &App{ID: 123456, Key: key(t), Now: func() time.Time { return fixedNow }}
	tok, err := app.JWT()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d parts", len(parts))
	}

	var header map[string]string
	decode(t, parts[0], &header)
	if header["alg"] != "RS256" || header["typ"] != "JWT" {
		t.Fatalf("bad header: %v", header)
	}
	var claims map[string]int64
	decode(t, parts[1], &claims)
	if claims["iss"] != 123456 || claims["iat"] != fixedNow.Add(-time.Minute).Unix() || claims["exp"] != fixedNow.Add(9*time.Minute).Unix() {
		t.Fatalf("bad claims: %v", claims)
	}
	if claims["exp"]-claims["iat"] > 600 {
		t.Fatal("jwt lives longer than GitHub's 10 minute limit")
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key(t).PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("signature does not verify: %v", err)
	}
}

func TestJWTNeedsIDAndKey(t *testing.T) {
	for _, app := range []*App{{ID: 1}, {Key: key(t)}} {
		if _, err := app.JWT(); err == nil {
			t.Fatalf("expected error for %+v", app)
		}
	}
}

func decode(t *testing.T, part string, v any) {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

// fakeGitHub checks requests the way GitHub would and records what it saw.
func fakeGitHub(t *testing.T, status int) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") == "" {
			http.Error(w, "missing headers", http.StatusBadRequest)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/55501234/access_tokens":
			auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			parts := strings.Split(auth, ".")
			sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
			if rsa.VerifyPKCS1v15(&key(t).PublicKey, crypto.SHA256, sum[:], sig) != nil {
				http.Error(w, "bad jwt", http.StatusUnauthorized)
				return
			}
			body, _ := io.ReadAll(r.Body)
			if !bytes.Contains(body, []byte(`"contents":"read"`)) || bytes.Contains(body, []byte("write")) {
				http.Error(w, "must ask for read-only contents", http.StatusBadRequest)
				return
			}
			w.WriteHeader(status)
			fmt.Fprint(w, `{"token":"ghs_realsecretvalue","expires_at":"2026-10-10T10:00:00Z"}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/octo-dev/foyr-hello/commits/"):
			if r.Header.Get("Authorization") != "Bearer ghs_realsecretvalue" {
				http.Error(w, "bad token", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(status)
			fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestInstallationTokenAndCheckCommit(t *testing.T) {
	srv, seen := fakeGitHub(t, http.StatusCreated)
	app := &App{ID: 123456, Key: key(t), BaseURL: srv.URL}
	tok, err := app.InstallationToken(context.Background(), 55501234)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "ghs_realsecretvalue" || !tok.ExpiresAt.Equal(time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected token: %q %v", tok.Value, tok.ExpiresAt)
	}

	okSrv, _ := fakeGitHub(t, http.StatusOK)
	app.BaseURL = okSrv.URL
	if err := app.CheckCommit(context.Background(), tok, "octo-dev/foyr-hello", "3f9c2e1a7b4d5c6e8f9012345678901234abcdef"); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 1 {
		t.Fatalf("requests: %v", *seen)
	}
}

func TestInstallationTokenErrors(t *testing.T) {
	srv, _ := fakeGitHub(t, http.StatusNotFound)
	app := &App{ID: 123456, Key: key(t), BaseURL: srv.URL}
	tests := []struct {
		name string
		id   int64
		want string
	}{
		{"github error", 55501234, "status 404"},
		{"unknown installation", 99, "status 404"},
		{"invalid id", 0, "invalid installation id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := app.InstallationToken(context.Background(), tt.id)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCheckCommitRejectsBadInput(t *testing.T) {
	app := &App{ID: 1, Key: key(t), BaseURL: "http://127.0.0.1:1"}
	for _, in := range [][2]string{
		{"octo-dev/foyr-hello/../../x", "3f9c2e1a7b4d5c6e8f9012345678901234abcdef"},
		{"octo-dev/foyr-hello", "main"},
		{"no-slash", "3f9c2e1a7b4d5c6e8f9012345678901234abcdef"},
	} {
		if err := app.CheckCommit(context.Background(), Token{Value: "x"}, in[0], in[1]); err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Fatalf("CheckCommit(%q, %q) = %v", in[0], in[1], err)
		}
	}
}

func TestTokenNeverPrintsItsValue(t *testing.T) {
	tok := Token{Value: "ghs_realsecretvalue"}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("got token", "token", tok)
	out := fmt.Sprintf("%v %s %+v %#v", tok, tok, tok, tok) + buf.String()
	if strings.Contains(out, "realsecret") {
		t.Fatalf("token value leaked: %s", out)
	}
}
