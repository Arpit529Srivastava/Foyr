package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Arpit529Srivastava/Foyr/internal/config"
	"github.com/Arpit529Srivastava/Foyr/internal/db/dbtest"
	"github.com/Arpit529Srivastava/Foyr/internal/github"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

var ctx = context.Background()

// syncBuffer is a log sink that is safe for concurrent writes.
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

type env struct {
	h      http.Handler
	st     *store.Store
	logs   *syncBuffer
	secret []byte
}

func setup(t *testing.T) env {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("a-long-random-webhook-secret")
	cfg := &config.Config{GitHubAppID: 123456, GitHubPrivateKey: key, GitHubWebhookSecret: secret}
	st := store.New(dbtest.New(t))
	logs := &syncBuffer{}
	return env{h: newServer(cfg, obs.NewLogger(logs), st), st: st, logs: logs, secret: secret}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../internal/github/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (e env) post(event, delivery string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-GitHub-Delivery", delivery)
	req.Header.Set("X-Hub-Signature-256", github.Sign(e.secret, body))
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.st.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPushQueuesTracedDeployment(t *testing.T) {
	e := setup(t)
	rec := e.post("push", "d-1", fixture(t, "push.json"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	trace := rec.Header().Get("X-Request-Id")

	d, err := e.st.GetDeployment(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.CommitSHA != "3f9c2e1a7b4d5c6e8f9012345678901234abcdef" || d.Branch != "main" || d.PusherLogin != "octo-dev" ||
		d.CommitMessage != "Update page" || d.DeliveryID != "d-1" || d.TraceID != trace || d.Status != store.StatusQueued {
		t.Fatalf("deployment = %+v (trace %s)", d, trace)
	}
	if n := e.count(t, "SELECT count(*) FROM jobs WHERE status = 'queued' AND trace_id = $1", trace); n != 1 {
		t.Fatalf("queued jobs with trace = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM webhook_deliveries WHERE delivery_id = 'd-1' AND result = 'ok'
		AND detail = 'deployment 1 queued' AND trace_id = $1 AND installation_id = 55501234`, trace); n != 1 {
		t.Fatal("delivery not recorded")
	}
	if n := e.count(t, "SELECT count(*) FROM audit_events WHERE action = 'deployment.created' AND trace_id = $1", trace); n != 1 {
		t.Fatal("audit event missing")
	}
	for _, want := range []string{`"msg":"deployment queued"`, `"deploy_id":1`, `"request_id":"` + trace} {
		if !strings.Contains(e.logs.String(), want) {
			t.Errorf("logs missing %s", want)
		}
	}
}

func TestRedeliveredPushBuildsOnce(t *testing.T) {
	e := setup(t)
	body := fixture(t, "push.json")
	e.post("push", "d-1", body)
	if rec := e.post("push", "d-1", body); rec.Code != http.StatusAccepted {
		t.Fatalf("redelivery status = %d", rec.Code)
	}
	if n := e.count(t, "SELECT count(*) FROM deployments"); n != 1 {
		t.Fatalf("deployments = %d, want 1", n)
	}
	if n := e.count(t, "SELECT count(*) FROM webhook_deliveries WHERE attempts = 2 AND result = 'ignored'"); n != 1 {
		t.Fatal("redelivery not counted as ignored")
	}
}

func TestPushesThatDoNotDeploy(t *testing.T) {
	push := string(fixture(t, "push.json"))
	tests := []struct {
		name, body, reason string
	}{
		{"deleted branch", string(fixture(t, "push_delete.json")), "branch deleted"},
		{"tag", strings.Replace(push, "refs/heads/main", "refs/tags/v1.0.0", 1), "not a branch"},
		{"other branch", strings.Replace(push, "refs/heads/main", "refs/heads/feature", 1), "not the default branch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := setup(t)
			if rec := e.post("push", "d-x", []byte(tt.body)); rec.Code != http.StatusAccepted {
				t.Fatalf("status = %d", rec.Code)
			}
			if n := e.count(t, "SELECT count(*) FROM deployments"); n != 0 {
				t.Fatalf("deployments = %d, want 0", n)
			}
			if n := e.count(t, "SELECT count(*) FROM webhook_deliveries WHERE result = 'ignored' AND detail = $1", tt.reason); n != 1 {
				t.Fatalf("delivery with reason %q not recorded", tt.reason)
			}
		})
	}
}

func TestInstallationStoresApps(t *testing.T) {
	e := setup(t)
	if rec := e.post("installation", "d-2", fixture(t, "installation.json")); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	if n := e.count(t, `SELECT count(*) FROM apps WHERE full_name = 'octo-dev/foyr-hello' AND installation_id = 55501234
		AND removed_at IS NULL`); n != 1 {
		t.Fatal("app not stored")
	}
	if n := e.count(t, `SELECT count(*) FROM audit_events WHERE action IN ('installation.created', 'app.connected')
		AND actor_login = 'octo-dev'`); n != 2 {
		t.Fatalf("audit events = %d, want 2", n)
	}
}

func TestDatabaseDownAnswers500(t *testing.T) {
	e := setup(t)
	e.st.Pool.Close()
	if rec := e.post("push", "d-1", fixture(t, "push.json")); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 so GitHub can redeliver", rec.Code)
	}
	for _, want := range []string{`"msg":"webhook failed"`, `"msg":"record delivery failed"`} {
		if !strings.Contains(e.logs.String(), want) {
			t.Errorf("logs missing %s", want)
		}
	}
}

func TestMaintainOnce(t *testing.T) {
	e := setup(t)
	maintainOnce(ctx, e.st, obs.NewLogger(e.logs))
	if n := e.count(t, "SELECT count(*) FROM pg_inherits WHERE inhparent = 'audit_events'::regclass"); n != partitionsAhead+2 {
		t.Fatalf("audit partitions = %d, want %d (default + %d months)", n, partitionsAhead+2, partitionsAhead+1)
	}
	if !strings.Contains(e.logs.String(), `"msg":"maintenance done"`) {
		t.Fatal("maintenance did not finish")
	}
}

func TestRoutes(t *testing.T) {
	e := setup(t)
	for _, tt := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/webhooks/github", http.StatusMethodNotAllowed},
		{http.MethodGet, "/nope", http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != tt.want {
			body, _ := io.ReadAll(rec.Body)
			t.Errorf("%s %s = %d, want %d (%s)", tt.method, tt.path, rec.Code, tt.want, body)
		}
	}
}

func TestOwner(t *testing.T) {
	for in, want := range map[string]string{"octo-dev/foyr-hello": "octo-dev", "solo": "solo", "": ""} {
		if got := owner(in); got != want {
			t.Errorf("owner(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFounderPageIsOffWithoutAKey(t *testing.T) {
	e := setup(t)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/founder", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /founder without a key = %d, want 404", rec.Code)
	}

	cfg := &config.Config{GitHubWebhookSecret: e.secret, FounderKey: strings.Repeat("k", 32)}
	rec = httptest.NewRecorder()
	newServer(cfg, obs.NewLogger(e.logs), e.st).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/founder", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `action="/founder/login"`) {
		t.Fatalf("GET /founder with a key = %d", rec.Code)
	}
}
