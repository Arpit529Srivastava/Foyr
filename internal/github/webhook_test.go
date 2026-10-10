package github

import (
	"bytes"
	"context"
	"errors"
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
	push       []PushEvent
	install    []InstallationEvent
	deliveries []Delivery
	err        error
}

func newHandler(c *calls) *Handler {
	return &Handler{
		Secret: testSecret,
		OnPush: func(_ context.Context, _ string, e PushEvent) (Outcome, error) {
			c.push = append(c.push, e)
			return Outcome{Result: "ok", Detail: "queued"}, c.err
		},
		OnInstall: func(_ context.Context, _ string, e InstallationEvent) (Outcome, error) {
			c.install = append(c.install, e)
			return Outcome{Result: "ok"}, c.err
		},
		Record: func(_ context.Context, d Delivery) { c.deliveries = append(c.deliveries, d) },
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
		e.Repository.FullName != "octo-dev/foyr-hello" || e.Installation.ID != 55501234 || !e.Repository.Private || e.Deleted ||
		e.Sender.Login != "octo-dev" || e.Sender.ID != 1 || e.HeadCommit == nil || e.HeadCommit.Message != "Update page" {
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
		len(e.Repositories) != 1 || e.Repositories[0].FullName != "octo-dev/foyr-hello" || !e.Repositories[0].Private ||
		e.Sender.Login != "octo-dev" {
		t.Fatalf("unexpected installation event: %+v", e)
	}
}

func TestWebhookRecordsDeliveries(t *testing.T) {
	push, ping := load(t, "push.json"), load(t, "ping.json")
	broken := []byte(`{"ref":`)
	tests := []struct {
		name, event string
		body        []byte
		err         error
		wantStatus  int
		want        Delivery
	}{
		{"push ok", "push", push, nil, http.StatusAccepted,
			Delivery{Event: "push", InstallationID: 55501234, Repo: "octo-dev/foyr-hello", Result: "ok", Detail: "queued"}},
		{"callback error answers 500", "push", push, errors.New("database down"), http.StatusInternalServerError,
			Delivery{Event: "push", InstallationID: 55501234, Repo: "octo-dev/foyr-hello", Result: "error", Detail: "database down"}},
		{"ping", "ping", ping, nil, http.StatusAccepted, Delivery{Event: "ping", Result: "ok", Detail: "ping"}},
		{"unknown event", "star", ping, nil, http.StatusAccepted,
			Delivery{Event: "star", Result: "ignored", Detail: "event not handled"}},
		{"broken json", "push", broken, nil, http.StatusBadRequest,
			Delivery{Event: "push", Result: "error", Detail: "invalid payload"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := calls{err: tt.err}
			rec := send(newHandler(&c), http.MethodPost, tt.event, tt.body, Sign(testSecret, tt.body))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if len(c.deliveries) != 1 {
				t.Fatalf("recorded %d deliveries, want 1", len(c.deliveries))
			}
			got := c.deliveries[0]
			if got.ID != "72d3162e-cc78-11e3-81ab-4c9367dc0958" || got.Duration <= 0 {
				t.Fatalf("delivery id/duration = %q %v", got.ID, got.Duration)
			}
			got.ID, got.Duration = "", 0
			if got != tt.want {
				t.Fatalf("delivery = %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestForgedWebhookIsNotRecorded(t *testing.T) {
	var c calls
	push := load(t, "push.json")
	send(newHandler(&c), http.MethodPost, "push", push, Sign([]byte("wrong-secret-wrong-secret"), push))
	if len(c.deliveries) != 0 {
		t.Fatal("unverified request was recorded")
	}
}
