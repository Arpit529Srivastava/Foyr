package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
	"github.com/Arpit529Srivastava/Foyr/internal/config"
	"github.com/Arpit529Srivastava/Foyr/internal/db/dbtest"
	"github.com/Arpit529Srivastava/Foyr/internal/github"
	"github.com/Arpit529Srivastava/Foyr/internal/mail"
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

func TestDashboardNeedsTheFounderSignIn(t *testing.T) {
	e := setup(t)
	for _, p := range []string{"/dashboard", "/dashboard/octo-dev", "/dashboard/octo-dev/foyr-hello/settings"} {
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/founder" {
			t.Errorf("GET %s signed out = %d %q, want a redirect to /founder", p, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestLoginRoutesExistOnlyWhenLoginIsOn(t *testing.T) {
	e := setup(t)
	get := func(h http.Handler, host, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "https://"+host+path, nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := get(e.h, "auth.foyr.test", "/login"); rec.Code != http.StatusNotFound {
		t.Fatalf("login off: GET /login = %d, want 404", rec.Code)
	}

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	cfg := &config.Config{GitHubWebhookSecret: e.secret, SessionKey: priv, AuthURL: "https://auth.foyr.test", AppsDomain: "apps.foyr.test"}
	on := newServer(cfg, obs.NewLogger(e.logs), e.st)
	rec := get(on, "auth.foyr.test", "/login?app=foyr-hello&sso=0")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `name="email"`) {
		t.Fatalf("login on: GET /login = %d %s", rec.Code, rec.Body)
	}
	// Without sso=0 the page hops to dashboard SSO on the dashboard host (ADR-039); here that is app.foyr.test.
	rec = get(on, "auth.foyr.test", "/login?app=foyr-hello")
	if want := "https://app.foyr.test/dashboard/sso?app=foyr-hello&return=%2F"; rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
		t.Fatalf("login on: GET /login?app = %d %q, want 303 %q", rec.Code, rec.Header().Get("Location"), want)
	}
	// A visitor with no dashboard session ends on the email form after the hop, in one step and with no loop.
	rec = get(on, "app.foyr.test", "/dashboard/sso?app=foyr-hello&return=/")
	back := "https://auth.foyr.test/login?app=foyr-hello&return=%2F&sso=0"
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != back {
		t.Fatalf("signed-out SSO = %d %q, want 303 %q", rec.Code, rec.Header().Get("Location"), back)
	}
	if rec = get(on, "auth.foyr.test", "/login?app=foyr-hello&return=%2F&sso=0"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `name="email"`) {
		t.Fatalf("email form after a failed hop = %d %s", rec.Code, rec.Body)
	}
	if rec := get(on, "api.foyr.test", "/login"); rec.Code != http.StatusNotFound {
		t.Errorf("login on the api host = %d, want 404 (only the auth host serves it)", rec.Code)
	}
}

func assertionKeys(t *testing.T, n int) []*ecdsa.PublicKey {
	t.Helper()
	var csv []string
	for i := 0; i < n; i++ {
		_, pub := authn.NewAssertionKeyPair()
		csv = append(csv, pub)
	}
	keys, err := authn.ParseAssertionPublicKeys(strings.Join(csv, ","))
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func keySetServer(t *testing.T, e *env, keys []*ecdsa.PublicKey, loginOn bool) http.Handler {
	t.Helper()
	cfg := &config.Config{GitHubWebhookSecret: e.secret, AuthURL: "https://auth.test", AppsDomain: "apps.test", AssertionPublicKeys: keys}
	if loginOn {
		_, priv, _ := ed25519.GenerateKey(rand.Reader)
		cfg.SessionKey = priv
	}
	return newServer(cfg, obs.NewLogger(e.logs), e.st)
}

func keySetReq(h http.Handler, method, host string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://"+host+"/.well-known/jwks.json", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestKeySetServedOnTheAuthHostOnly(t *testing.T) {
	e := setup(t)
	keys := assertionKeys(t, 1)
	on := keySetServer(t, &e, keys, true)

	rec := keySetReq(on, http.MethodGet, "auth.test")
	if rec.Code != http.StatusOK || rec.Body.String() != string(authn.JWKS(keys)) {
		t.Fatalf("auth host = %d %s", rec.Code, rec.Body)
	}
	for k, v := range map[string]string{
		"Content-Type":                "application/json",
		"Cache-Control":               "public, max-age=300",
		"Access-Control-Allow-Origin": "*",
	} {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if rec := keySetReq(on, http.MethodGet, "AUTH.test"); rec.Code != http.StatusOK {
		t.Errorf("upper-case host = %d, want 200", rec.Code)
	}
	for _, host := range []string{"app.test", "api.test"} {
		if rec := keySetReq(on, http.MethodGet, host); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", host, rec.Code)
		}
	}
	head := keySetReq(on, http.MethodHead, "auth.test")
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Cache-Control") != "public, max-age=300" ||
		head.Header().Get("Content-Type") != "application/json" || head.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("HEAD = %d %q %v", head.Code, head.Body, head.Header())
	}
	if rec := keySetReq(on, http.MethodPost, "auth.test"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", rec.Code)
	}

	off := keySetServer(t, &e, keys, false)
	for _, host := range []string{"auth.test", "app.test", "api.test"} {
		if rec := keySetReq(off, http.MethodGet, host); rec.Code != http.StatusNotFound {
			t.Errorf("login off, %s = %d, want 404", host, rec.Code)
		}
	}
}

func TestKeySetListsEveryConfiguredKey(t *testing.T) {
	e := setup(t)
	keys := assertionKeys(t, 2)
	if authn.KeyID(keys[0]) < authn.KeyID(keys[1]) {
		keys[0], keys[1] = keys[1], keys[0]
	}
	rec := keySetReq(keySetServer(t, &e, keys, true), http.MethodGet, "auth.test")
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &set); err != nil || len(set.Keys) != 2 {
		t.Fatalf("body = %s err=%v", rec.Body, err)
	}
	if set.Keys[0].Kid >= set.Keys[1].Kid {
		t.Errorf("kids not sorted: %v", set.Keys)
	}
}

func TestMailerChoice(t *testing.T) {
	log := obs.NewLogger(&syncBuffer{})
	if m := mailer(&config.Config{}, log); m != nil {
		t.Errorf("login off: mailer = %T, want nil", m)
	}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	dev := &config.Config{SessionKey: priv, AuthURL: "http://localhost:8080"}
	if _, ok := mailer(dev, log).(mail.LogSender); !ok {
		t.Errorf("localhost without a Resend key: mailer = %T, want LogSender", mailer(dev, log))
	}
	prod := &config.Config{SessionKey: priv, AuthURL: "https://auth.foyr.test", ResendAPIKey: "k", MailFrom: "Foyr <l@foyr.test>"}
	if r, ok := mailer(prod, log).(mail.Resend); !ok || r.From != "Foyr <l@foyr.test>" {
		t.Errorf("with a Resend key: mailer = %#v", mailer(prod, log))
	}
}

func TestSiteURLComesFromTheAuthURL(t *testing.T) {
	for auth, want := range map[string]string{
		"https://auth.foyr.online":  "https://foyr.online",
		"https://auth.example.com":  "https://example.com",
		"http://localhost:8080":     "",
		"https://login.example.com": "",
		"":                          "",
	} {
		if got := siteURL(&config.Config{AuthURL: auth}); got != want {
			t.Errorf("siteURL(%q) = %q, want %q", auth, got, want)
		}
	}
}

func TestFoyrdServesTheFavicon(t *testing.T) {
	e := setup(t)
	for path, ctype := range map[string]string{"/favicon.svg": "image/svg+xml", "/favicon.png": "image/png", "/favicon.ico": "image/png"} {
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != ctype {
			t.Errorf("%s: %d %q, want 200 %q", path, rec.Code, rec.Header().Get("Content-Type"), ctype)
		}
	}
}

func TestDashboardURLComesFromTheAuthURL(t *testing.T) {
	for auth, want := range map[string]string{
		"https://auth.foyr.online":  "https://app.foyr.online",
		"http://localhost:8080":     "http://localhost:8080",
		"https://login.example.com": "",
		"":                          "",
	} {
		if got := dashboardURL(&config.Config{AuthURL: auth}); got != want {
			t.Errorf("dashboardURL(%q) = %q, want %q", auth, got, want)
		}
	}
}

// A linked customer signs in through the real routes and store, and sees only their own account.
func TestCustomerSignsInToTheDashboard(t *testing.T) {
	e := setup(t)
	if rec := e.post("push", "d-1", fixture(t, "push.json")); rec.Code != http.StatusAccepted {
		t.Fatalf("push = %d", rec.Code)
	}
	mine, err := e.st.ListInstallations(ctx)
	if err != nil || len(mine) != 1 {
		t.Fatalf("installations = %v, %v", mine, err)
	}
	if _, err := e.st.Pool.Exec(ctx, `INSERT INTO installations (id, account_login) VALUES (999, 'other-co')`); err != nil {
		t.Fatal(err)
	}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	cfg := &config.Config{GitHubWebhookSecret: e.secret, SessionKey: priv, AuthURL: "https://auth.foyr.test", AppsDomain: "apps.foyr.test",
		FounderKey: "0123456789abcdef0123456789abcdef"}
	h := newServer(cfg, obs.NewLogger(e.logs), e.st)

	send := func(method, host, path, form string, c *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://"+host+path, strings.NewReader(form))
		req.Host = host
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", "https://"+host)
		}
		if c != nil {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	const app = "app.foyr.test"

	if rec := send("GET", app, "/dashboard", "", nil); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/dashboard/login" {
		t.Fatalf("signed out: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := send("GET", "api.foyr.test", "/dashboard", "", nil); rec.Header().Get("Location") != "/founder" {
		t.Errorf("signed out on the api host: %q, want /founder", rec.Header().Get("Location"))
	}
	if rec := send("GET", app, "/", "", nil); rec.Header().Get("Location") != "/dashboard" {
		t.Errorf("app host root: %q", rec.Header().Get("Location"))
	}

	// Only a linked email gets a token.
	if _, ok, _ := e.st.StartDashboardLogin(ctx, "ana@acme.test", time.Now()); ok {
		t.Fatal("an unlinked email got a token")
	}
	if _, err := e.st.LinkAccountMember(ctx, mine[0].ID, "ana@acme.test", "owner", store.Actor{Type: "system", Login: "founder"}); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := e.st.StartDashboardLogin(ctx, "ana@acme.test", time.Now())
	if err != nil || !ok {
		t.Fatalf("StartDashboardLogin = %v, %v", ok, err)
	}
	if rec := send("GET", app, "/dashboard/login/verify?t="+raw, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("Continue page = %d", rec.Code)
	}
	rec := send("POST", app, "/dashboard/login/verify", "t="+raw, nil)
	cs := rec.Result().Cookies()
	if rec.Code != http.StatusSeeOther || len(cs) != 1 || cs[0].Name != "__Host-foyr_dash" {
		t.Fatalf("verify = %d %v", rec.Code, cs)
	}
	c := cs[0]

	if rec := send("GET", app, "/dashboard", "", c); rec.Header().Get("Location") != "/dashboard/"+mine[0].Login {
		t.Fatalf("start = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	page := send("GET", app, "/dashboard/"+mine[0].Login, "", c)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "other-co") || !strings.Contains(page.Body.String(), "/dashboard/logout") {
		t.Fatalf("account page = %d\n%s", page.Code, page.Body)
	}
	if rec := send("GET", app, "/dashboard/other-co", "", c); rec.Code != http.StatusNotFound {
		t.Errorf("another account = %d, want 404", rec.Code)
	}
	// The customer cookie opens nothing of the founder's.
	if rec := send("GET", "api.foyr.test", "/founder/accounts", "", c); rec.Header().Get("Location") != "/founder" {
		t.Errorf("customer cookie on the founder page: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if out := send("POST", app, "/dashboard/logout", "", c); out.Code != http.StatusSeeOther {
		t.Fatalf("logout = %d", out.Code)
	}
	if rec := send("GET", app, "/dashboard/"+mine[0].Login, "", c); rec.Header().Get("Location") != "/dashboard/login" {
		t.Errorf("after logout: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}
