package github

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

var testSecret = []byte("a-long-random-webhook-secret")

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type calls struct {
	push    []PushEvent
	install []InstallationEvent
}

func newHandler(c *calls) *Handler {
	return &Handler{
		Secret:    testSecret,
		OnPush:    func(_ context.Context, _ string, e PushEvent) { c.push = append(c.push, e) },
		OnInstall: func(_ context.Context, _ string, e InstallationEvent) { c.install = append(c.install, e) },
	}
}

func send(h http.Handler, method, event string, body []byte, sig string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-GitHub-Delivery", "72d3162e-cc78-11e3-81ab-4c9367dc0958")
	if sig != "" {
		req.Header.Set("X-Hub-Signature-256", sig)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestWebhook(t *testing.T) {
	push, install, ping := load(t, "push.json"), load(t, "installation.json"), load(t, "ping.json")
	broken := []byte(`{"ref":`)

	tests := []struct {
		name        string
		method      string
		event       string
		body        []byte
		sig         string
		wantStatus  int
		wantPush    int
		wantInstall int
	}{
		{"push", http.MethodPost, "push", push, Sign(testSecret, push), http.StatusAccepted, 1, 0},
		{"installation", http.MethodPost, "installation", install, Sign(testSecret, install), http.StatusAccepted, 0, 1},
		{"ping", http.MethodPost, "ping", ping, Sign(testSecret, ping), http.StatusAccepted, 0, 0},
		{"unknown event is ignored", http.MethodPost, "star", ping, Sign(testSecret, ping), http.StatusAccepted, 0, 0},
		{"bad signature", http.MethodPost, "push", push, Sign([]byte("wrong-secret-wrong-secret"), push), http.StatusUnauthorized, 0, 0},
		{"missing signature", http.MethodPost, "push", push, "", http.StatusUnauthorized, 0, 0},
		{"GET not allowed", http.MethodGet, "push", nil, "", http.StatusMethodNotAllowed, 0, 0},
		{"broken json", http.MethodPost, "push", broken, Sign(testSecret, broken), http.StatusBadRequest, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c calls
			rec := send(newHandler(&c), tt.method, tt.event, tt.body, tt.sig)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if len(c.push) != tt.wantPush || len(c.install) != tt.wantInstall {
				t.Fatalf("push calls = %d, install calls = %d", len(c.push), len(c.install))
			}
		})
	}
}

func TestWebhookOversizedBody(t *testing.T) {
	var c calls
	body := bytes.Repeat([]byte("a"), maxBody+1)
	rec := send(newHandler(&c), http.MethodPost, "push", body, Sign(testSecret, body))
	if rec.Code != http.StatusRequestEntityTooLarge || len(c.push) != 0 {
		t.Fatalf("status = %d, push calls = %d", rec.Code, len(c.push))
	}
}

func TestPushEventFields(t *testing.T) {
	var c calls
	body := load(t, "push.json")
	send(newHandler(&c), http.MethodPost, "push", body, Sign(testSecret, body))
	e := c.push[0]
	if e.Branch() != "main" || e.After != "3f9c2e1a7b4d5c6e8f9012345678901234abcdef" ||
		e.Repository.FullName != "octo-dev/foyr-hello" || e.Installation.ID != 55501234 || !e.Repository.Private || e.Deleted {
		t.Fatalf("unexpected push event: %+v", e)
	}
}

func TestBranchDeletionIsFlagged(t *testing.T) {
	var c calls
	body := load(t, "push_delete.json")
	send(newHandler(&c), http.MethodPost, "push", body, Sign(testSecret, body))
	if !c.push[0].Deleted {
		t.Fatal("branch deletion not flagged")
	}
}

func TestTagPushHasNoBranch(t *testing.T) {
	if b := (PushEvent{Ref: "refs/tags/v1.0.0"}).Branch(); b != "" {
		t.Fatalf("Branch() = %q for a tag", b)
	}
}

func TestInstallationEventFields(t *testing.T) {
	var c calls
	body := load(t, "installation.json")
	send(newHandler(&c), http.MethodPost, "installation", body, Sign(testSecret, body))
	e := c.install[0]
	if e.Action != "created" || e.Installation.ID != 55501234 || e.Installation.Account.Login != "octo-dev" ||
		len(e.Repositories) != 1 || e.Repositories[0].FullName != "octo-dev/foyr-hello" {
		t.Fatalf("unexpected installation event: %+v", e)
	}
}
