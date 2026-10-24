package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendSends(t *testing.T) {
	var got map[string]any
	var auth, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, ctype = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"abc"}`))
	}))
	defer srv.Close()

	r := Resend{APIKey: "re_secret_123", From: "Foyr <login@foyr.test>", URL: srv.URL}
	err := r.Send(context.Background(), Message{To: "ana@example.com", Subject: "Sign in to crm", Text: "plain", HTML: "<p>html</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer re_secret_123" || ctype != "application/json" {
		t.Errorf("headers: Authorization %q, Content-Type %q", auth, ctype)
	}
	to, _ := got["to"].([]any)
	if got["from"] != "Foyr <login@foyr.test>" || len(to) != 1 || to[0] != "ana@example.com" ||
		got["subject"] != "Sign in to crm" || got["text"] != "plain" || got["html"] != "<p>html</p>" {
		t.Errorf("body = %v", got)
	}
}

func TestResendErrorHasStatusNotKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"invalid from address"}`))
	}))
	defer srv.Close()

	r := Resend{APIKey: "re_secret_123", From: "x", URL: srv.URL}
	err := r.Send(context.Background(), Message{To: "a@b.c", Subject: "s", Text: "t"})
	if err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("err = %v, want it to name status 422", err)
	}
	if strings.Contains(err.Error(), "re_secret_123") {
		t.Errorf("error leaks the API key: %v", err)
	}
}

func TestResendUnreachableErrorHasNoKey(t *testing.T) {
	r := Resend{APIKey: "re_secret_123", From: "x", URL: "http://127.0.0.1:1"}
	err := r.Send(context.Background(), Message{To: "a@b.c", Subject: "s", Text: "t"})
	if err == nil || strings.Contains(err.Error(), "re_secret_123") {
		t.Fatalf("err = %v", err)
	}
}

func TestLogSender(t *testing.T) {
	var buf bytes.Buffer
	s := LogSender{Log: slog.New(slog.NewTextHandler(&buf, nil))}
	if err := s.Send(context.Background(), Message{To: "ana@example.com", Subject: "Sign in", Text: "open https://x/y", HTML: "<b>no</b>"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"ana@example.com", "Sign in", "open https://x/y"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q: %s", want, out)
		}
	}
	if strings.Contains(out, "<b>") {
		t.Errorf("log includes the HTML body: %s", out)
	}
}
