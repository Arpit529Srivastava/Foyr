package proxy

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/obs"
)

func newProxy(t *testing.T, routes map[string]string) http.Handler {
	t.Helper()
	tbl := &Table{}
	if routes != nil {
		tbl.Set(routes)
	}
	return obs.RequestID(obs.NewLogger(io.Discard), NewHandler("apps.foyr.online", tbl))
}

func do(h http.Handler, host, path string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProxiesToLiveDeployment(t *testing.T) {
	var got *http.Request
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		w.Header().Set("X-From", "app")
		io.WriteString(w, "hello from foyr-hello")
	}))
	defer app.Close()
	h := newProxy(t, map[string]string{"foyr-hello": strings.TrimPrefix(app.URL, "http://")})

	rec := do(h, "Foyr-Hello.apps.foyr.online:443", "/dashboard?x=1", map[string]string{
		"X-Foyr-User-Id":    "usr_forged",
		"X-Foyr-Role":       "owner",
		"X-Forwarded-For":   "203.0.113.9",
		"X-Forwarded-Proto": "https",
	})
	if rec.Code != http.StatusOK || rec.Body.String() != "hello from foyr-hello" || rec.Header().Get("X-From") != "app" {
		t.Fatalf("response = %d %q", rec.Code, rec.Body)
	}
	if got.URL.Path != "/dashboard" || got.URL.RawQuery != "x=1" || got.Host != "Foyr-Hello.apps.foyr.online:443" {
		t.Fatalf("app saw %s %s?%s", got.Host, got.URL.Path, got.URL.RawQuery)
	}
	if got.Header.Get("X-Foyr-User-Id") != "" || got.Header.Get("X-Foyr-Role") != "" {
		t.Fatal("forged X-Foyr-* headers reached the app")
	}
	if xff := got.Header.Get("X-Forwarded-For"); !strings.HasPrefix(xff, "203.0.113.9, ") {
		t.Fatalf("X-Forwarded-For = %q, want the visitor first", xff)
	}
	if got.Header.Get("X-Forwarded-Proto") != "https" || got.Header.Get("X-Request-Id") == "" {
		t.Fatalf("proto=%q request id=%q", got.Header.Get("X-Forwarded-Proto"), got.Header.Get("X-Request-Id"))
	}
}

func TestNoAppPages(t *testing.T) {
	h := newProxy(t, map[string]string{"foyr-hello": "127.0.0.1:1"})
	for _, host := range []string{
		"missing.apps.foyr.online", // no such app
		"admin.apps.foyr.online",   // reserved
		"a.b.apps.foyr.online",     // nested
		"apps.foyr.online",         // the bare domain
		"foyr-hello.apps.evil.com", // other domain
		"foyr-hello.apps.foyr.online.evil.com",
		"bad_slug.apps.foyr.online",
	} {
		rec := do(h, host, "/", nil)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "No app at this address") {
			t.Errorf("%s: %d %q", host, rec.Code, rec.Body.String()[:min(60, rec.Body.Len())])
		}
	}
}

func TestPagesEscapeInput(t *testing.T) {
	var b bytes.Buffer
	rec := httptest.NewRecorder()
	page(rec, 404, "<script>", `"><img>`)
	b.Write(rec.Body.Bytes())
	if strings.Contains(b.String(), "<script>") || strings.Contains(b.String(), "<img>") {
		t.Fatal("page did not escape its text")
	}
}

func TestAppDownIs502(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	rec := do(newProxy(t, map[string]string{"foyr-hello": addr}), "foyr-hello.apps.foyr.online", "/", nil)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "Foyr · 502") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestHealthPath(t *testing.T) {
	if rec := do(newProxy(t, nil), "10.0.1.5:8081", HealthPath, nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("before routes load: %d, want 503", rec.Code)
	}
	if rec := do(newProxy(t, map[string]string{}), "10.0.1.5:8081", HealthPath, nil); rec.Code != http.StatusOK {
		t.Fatalf("after routes load: %d, want 200", rec.Code)
	}
}

func TestPollerLoadsAndKeepsLastGood(t *testing.T) {
	var fail atomic.Bool
	var auth atomic.Value
	foyrd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		if fail.Load() {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		io.WriteString(w, `{"routes":[{"slug":"foyr-hello","service":"d-4"},{"slug":"","service":"d-9"}]}`)
	}))
	defer foyrd.Close()
	tbl := &Table{}
	p := &Poller{URL: foyrd.URL, Token: "tok", Namespace: "apps.foyr.internal", Port: 3000,
		Every: 5 * time.Millisecond, Table: tbl, Log: obs.NewLogger(io.Discard)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for !tbl.Ready() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if up, ok := tbl.Lookup("foyr-hello"); !ok || up != "d-4.apps.foyr.internal:3000" || tbl.Len() != 1 {
		t.Fatalf("table = %q %v (len %d)", up, ok, tbl.Len())
	}
	if auth.Load() != "Bearer tok" {
		t.Fatalf("auth = %v", auth.Load())
	}
	fail.Store(true)
	time.Sleep(30 * time.Millisecond)
	if _, ok := tbl.Lookup("foyr-hello"); !ok {
		t.Fatal("a foyrd outage emptied the routing table; apps must keep serving")
	}
}

func TestParseStatic(t *testing.T) {
	m, err := ParseStatic("foyr-hello=localhost:3000, other=127.0.0.1:4000")
	if err != nil || m["foyr-hello"] != "localhost:3000" || m["other"] != "127.0.0.1:4000" {
		t.Fatalf("m=%v err=%v", m, err)
	}
	if _, err := ParseStatic("broken"); err == nil {
		t.Fatal("want error")
	}
}
