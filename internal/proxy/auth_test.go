package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
)

const (
	testAuthURL = "https://auth.foyr.test"
	testDomain  = "apps.foyr.test"
	testCookie  = "__Host-foyr_session"
)

type fakeExchanger struct {
	mu      sync.Mutex
	token   string
	exp     time.Time
	err     error
	codes   []string
	logouts []string
}

func (f *fakeExchanger) Exchange(_ context.Context, code, slug string) (string, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.codes = append(f.codes, code+"@"+slug)
	return f.token, f.exp, f.err
}

func (f *fakeExchanger) Logout(_ context.Context, sid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logouts = append(f.logouts, sid)
	return nil
}

type authRig struct {
	h      http.Handler
	access *Access
	exch   *fakeExchanger
	priv   ed25519.PrivateKey
	hits   int
	apub   *ecdsa.PublicKey // the half that verifies assertions
	auth   *Auth
}

const testPID = "usr_0123456789abcdef"

// echo is the app: it reports the identity headers and cookies it received.
type echo struct {
	Headers http.Header `json:"headers"`
	Cookie  string      `json:"cookie"`
	Path    string      `json:"path"`
}

func newAuthRig(t *testing.T) *authRig {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	akey, err := authn.ParseAssertionKey(mustKey())
	if err != nil {
		t.Fatal(err)
	}
	g := &authRig{priv: priv, apub: &akey.PublicKey, exch: &fakeExchanger{token: "TOK", exp: time.Now().Add(7 * 24 * time.Hour).Truncate(time.Second)}}
	g.exch.token = g.session("crm", 7, "cb")
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.hits++
		_ = json.NewEncoder(w).Encode(echo{Headers: r.Header, Cookie: r.Header.Get("Cookie"), Path: r.URL.RequestURI()})
	}))
	t.Cleanup(app.Close)
	up := strings.TrimPrefix(app.URL, "http://")
	tbl := &Table{}
	tbl.Set(map[string]string{"crm": up, "blog": up, "wiki": up})
	g.access = &Access{}
	g.access.Set(authn.AccessSnapshot{Apps: []authn.AppAccess{
		{Slug: "crm", Mode: "private", Members: map[int64]string{7: "admin"}},
		{Slug: "blog", Mode: "public", Members: map[int64]string{}},
		{Slug: "wiki", Mode: "private", Members: map[int64]string{}},
	}})
	hd := NewHandler(testDomain, tbl)
	hd.Access = g.access
	hd.Auth = &Auth{PublicKey: pub, AuthURL: testAuthURL, Exchanger: g.exch, AssertionKey: akey}
	g.auth = hd.Auth
	g.h = obs.RequestID(obs.NewLogger(io.Discard), hd)
	return g
}

func (g *authRig) session(slug string, uid int64, sid string) string {
	return g.sessionFor(slug, uid, sid, testPID)
}

func (g *authRig) sessionFor(slug string, uid int64, sid, pid string) string {
	return authn.Sign(g.priv, authn.Claims{SID: sid, UserID: uid, PublicID: pid, Email: "ana@example.com", Slug: slug, Exp: time.Now().Add(time.Hour).Unix()})
}

func mustKey() string {
	k, _ := authn.NewAssertionKeyPair()
	return k
}

type reqOpt func(*http.Request)

func withCookie(v string) reqOpt { return func(r *http.Request) { r.Header.Add("Cookie", v) } }
func withHeader(k, v string) reqOpt {
	return func(r *http.Request) { r.Header.Set(k, v) }
}

func (g *authRig) do(method, host, target string, opts ...reqOpt) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://"+host+target, nil)
	r.Host = host
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	g.h.ServeHTTP(w, r)
	return w
}

func seen(t *testing.T, w *httptest.ResponseRecorder) echo {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", w.Code, w.Body)
	}
	var e echo
	if err := json.NewDecoder(w.Body).Decode(&e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestPrivateAppRedirectsBrowsers(t *testing.T) {
	g := newAuthRig(t)
	w := g.do("GET", "crm."+testDomain, "/reports?q=1", withHeader("Accept", "text/html,application/xhtml+xml"))
	if w.Code != http.StatusFound {
		t.Fatalf("status %d, want 302", w.Code)
	}
	loc, _ := url.Parse(w.Header().Get("Location"))
	if loc.Scheme+"://"+loc.Host != testAuthURL || loc.Path != "/login" || loc.Query().Get("app") != "crm" || loc.Query().Get("return") != "/reports?q=1" {
		t.Fatalf("Location = %s", w.Header().Get("Location"))
	}
	if !strings.Contains(w.Header().Get("Location"), "return=%2Freports%3Fq%3D1") {
		t.Errorf("return is not escaped: %s", w.Header().Get("Location"))
	}
	if g.hits != 0 {
		t.Error("the app was reached without a session")
	}
}

func TestPrivateAppAPIGets401(t *testing.T) {
	g := newAuthRig(t)
	for name, w := range map[string]*httptest.ResponseRecorder{
		"json accept":       g.do("GET", "crm."+testDomain, "/api/x", withHeader("Accept", "application/json")),
		"no accept":         g.do("GET", "crm."+testDomain, "/api/x"),
		"post from browser": g.do("POST", "crm."+testDomain, "/api/x", withHeader("Accept", "text/html")),
	} {
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, w.Code)
			continue
		}
		var raw map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &raw)
		if raw["error"] != "login required" || !strings.HasPrefix(raw["login_url"], testAuthURL+"/login?app=crm") {
			t.Errorf("%s: body %s", name, w.Body)
		}
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Errorf("%s: Content-Type %q", name, w.Header().Get("Content-Type"))
		}
	}
}

func TestMemberIsForwarded(t *testing.T) {
	g := newAuthRig(t)
	tok := g.session("crm", 7, "s1")
	e := seen(t, g.do("GET", "crm."+testDomain, "/reports",
		withCookie("a=1; "+testCookie+"="+tok+"; theme=dark"),
		withHeader("X-Foyr-Role", "owner")))
	if e.Headers.Get("X-Foyr-User-Id") != testPID || e.Headers.Get("X-Foyr-Email") != "ana@example.com" || e.Headers.Get("X-Foyr-Role") != "admin" {
		t.Errorf("identity headers = %v", e.Headers)
	}
	if e.Cookie != "a=1; theme=dark" {
		t.Errorf("app saw cookies %q, want the session cookie removed and the others kept", e.Cookie)
	}
	if strings.Contains(strings.Join(e.Headers.Values("Cookie"), ";"), tok) {
		t.Error("the session token reached the app")
	}
}

func TestSessionCookieAloneIsRemovedEntirely(t *testing.T) {
	g := newAuthRig(t)
	e := seen(t, g.do("GET", "crm."+testDomain, "/", withCookie(testCookie+"="+g.session("crm", 7, "s1"))))
	if _, ok := e.Headers["Cookie"]; ok {
		t.Errorf("app still got a Cookie header: %v", e.Headers["Cookie"])
	}
}

func TestSpoofedHeadersStripped(t *testing.T) {
	g := newAuthRig(t)
	e := seen(t, g.do("GET", "blog."+testDomain, "/", withHeader("X-Foyr-Role", "owner"), withHeader("X-Foyr-User-Id", "1"), withHeader("x-foyr-assertion", "x")))
	for k := range e.Headers {
		if strings.HasPrefix(strings.ToLower(k), "x-foyr-") {
			t.Errorf("forged header %s reached the app", k)
		}
	}
}

func TestPublicAppIdentifiesSignedInVisitors(t *testing.T) {
	g := newAuthRig(t)
	anon := seen(t, g.do("GET", "blog."+testDomain, "/"))
	if anon.Headers.Get("X-Foyr-User-Id") != "" {
		t.Error("anonymous visitor got an identity")
	}
	e := seen(t, g.do("GET", "blog."+testDomain, "/", withCookie(testCookie+"="+g.session("blog", 42, "s9"))))
	if e.Headers.Get("X-Foyr-User-Id") != testPID || e.Headers.Get("X-Foyr-Role") != "viewer" {
		t.Errorf("signed-in visitor of a public app: %v", e.Headers)
	}
}

func TestSessionForOtherAppRejected(t *testing.T) {
	g := newAuthRig(t)
	// A session signed for crm, replayed on the private app wiki.
	w := g.do("GET", "wiki."+testDomain, "/", withHeader("Accept", "text/html"), withCookie(testCookie+"="+g.session("crm", 7, "s1")))
	if w.Code != http.StatusFound {
		t.Fatalf("status %d, want 302 to login (the session is for another app)", w.Code)
	}
	if g.hits != 0 {
		t.Error("the app was reached with another app's session")
	}
}

func TestBadAndExpiredSessionsAreNoSession(t *testing.T) {
	g := newAuthRig(t)
	expired := authn.Sign(g.priv, authn.Claims{SID: "s", UserID: 7, PublicID: testPID, Slug: "crm", Exp: time.Now().Add(-time.Minute).Unix()})
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	forged := authn.Sign(otherPriv, authn.Claims{SID: "s", UserID: 7, PublicID: testPID, Slug: "crm", Exp: time.Now().Add(time.Hour).Unix()})
	for name, tok := range map[string]string{"expired": expired, "forged": forged, "garbage": "v1.x.y"} {
		w := g.do("GET", "crm."+testDomain, "/", withHeader("Accept", "text/html"), withCookie(testCookie+"="+tok))
		if w.Code != http.StatusFound {
			t.Errorf("%s: status %d, want 302", name, w.Code)
		}
	}
	if g.hits != 0 {
		t.Error("the app was reached")
	}
}

func TestRevokedAndRemoved(t *testing.T) {
	g := newAuthRig(t)
	tok := g.session("crm", 7, "s1")
	get := func() *httptest.ResponseRecorder {
		return g.do("GET", "crm."+testDomain, "/", withHeader("Accept", "text/html"), withCookie(testCookie+"="+tok))
	}
	if w := get(); w.Code != http.StatusOK {
		t.Fatalf("member before: %d", w.Code)
	}

	snap := authn.AccessSnapshot{Apps: []authn.AppAccess{{Slug: "crm", Mode: "private", Members: map[int64]string{7: "admin"}}}, Revoked: []string{"s1"}}
	g.access.Set(snap)
	if w := get(); w.Code != http.StatusFound {
		t.Errorf("revoked session: %d, want 302 to login", w.Code)
	}

	g.access.Set(authn.AccessSnapshot{Apps: []authn.AppAccess{{Slug: "crm", Mode: "private", Members: map[int64]string{}}}})
	w := get()
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "You don&#39;t have access to crm") ||
		!strings.Contains(w.Body.String(), "ana@example.com") || !strings.Contains(w.Body.String(), `action="/__foyr/logout"`) {
		t.Errorf("removed member: %d %s", w.Code, w.Body)
	}
}

// A person who just signed in is a member in foyrd before authproxy's next poll knows it.
func TestFreshSessionWaitsForTheSnapshot(t *testing.T) {
	g := newAuthRig(t)
	sign := func(iat time.Time) string {
		return authn.Sign(g.priv, authn.Claims{SID: "s1", UserID: 99, PublicID: testPID, Email: "new@example.com", Slug: "crm",
			Iat: iat.Unix(), Exp: time.Now().Add(time.Hour).Unix()})
	}
	w := g.do("GET", "crm."+testDomain, "/", withHeader("Accept", "text/html"), withCookie(testCookie+"="+sign(time.Now().Add(-2*time.Second))))
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" ||
		!strings.Contains(w.Body.String(), `http-equiv="refresh"`) || strings.Contains(w.Body.String(), "have access") {
		t.Errorf("fresh non-member: %d %s; want a self-refreshing 503, not a 403", w.Code, w.Body)
	}
	// Once the snapshot knows the member, the same session gets in.
	g.access.Set(authn.AccessSnapshot{Apps: []authn.AppAccess{{Slug: "crm", Mode: "private", Members: map[int64]string{99: "member"}}}})
	if w := g.do("GET", "crm."+testDomain, "/", withCookie(testCookie+"="+sign(time.Now().Add(-2*time.Second)))); w.Code != http.StatusOK {
		t.Errorf("after the snapshot caught up: %d, want 200", w.Code)
	}
	// An old session of someone not on the app is a plain 403, with no refresh loop.
	g.access.Set(authn.AccessSnapshot{Apps: []authn.AppAccess{{Slug: "crm", Mode: "private", Members: map[int64]string{}}}})
	w = g.do("GET", "crm."+testDomain, "/", withHeader("Accept", "text/html"), withCookie(testCookie+"="+sign(time.Now().Add(-time.Hour))))
	if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "refresh") {
		t.Errorf("old non-member: %d %s; want a plain 403", w.Code, w.Body)
	}
}

func TestUnknownAppInSnapshotIsPrivate(t *testing.T) {
	g := newAuthRig(t)
	g.access.Set(authn.AccessSnapshot{}) // loaded, but knows no apps yet
	w := g.do("GET", "crm."+testDomain, "/", withHeader("Accept", "text/html"))
	if w.Code != http.StatusFound {
		t.Errorf("status %d, want 302: an app the snapshot doesn't know yet must not be open", w.Code)
	}
}

func TestNotReady(t *testing.T) {
	g := newAuthRig(t)
	g.h = func() http.Handler {
		tbl := &Table{}
		tbl.Set(map[string]string{"crm": "127.0.0.1:1"})
		hd := NewHandler(testDomain, tbl)
		hd.Access = &Access{}
		hd.Auth = &Auth{PublicKey: make([]byte, 32), AuthURL: testAuthURL, Exchanger: g.exch}
		return obs.RequestID(obs.NewLogger(io.Discard), hd)
	}()
	w := g.do("GET", "crm."+testDomain, "/", withHeader("Accept", "text/html"))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "Starting up") {
		t.Errorf("before the first snapshot: %d %s", w.Code, w.Body)
	}
	if w := g.do("GET", "crm."+testDomain, HealthPath); w.Code != http.StatusServiceUnavailable {
		t.Errorf("health before access loads: %d, want 503", w.Code)
	}
}

func TestHealthNeedsRoutesAndAccess(t *testing.T) {
	g := newAuthRig(t)
	if w := g.do("GET", "anything."+testDomain, HealthPath); w.Code != http.StatusOK {
		t.Errorf("health with both loaded: %d, want 200", w.Code)
	}
}

func TestCallback(t *testing.T) {
	g := newAuthRig(t)
	w := g.do("GET", "crm."+testDomain, "/__foyr/callback?code=abc&return=%2Freports")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/reports" {
		t.Fatalf("status %d Location %q", w.Code, w.Header().Get("Location"))
	}
	if len(g.exch.codes) != 1 || g.exch.codes[0] != "abc@crm" {
		t.Errorf("exchanged %v", g.exch.codes)
	}
	cs := w.Result().Cookies()
	if len(cs) != 1 {
		t.Fatalf("cookies = %v", cs)
	}
	c := cs[0]
	if c.Name != testCookie || c.Value != g.exch.token || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
		t.Errorf("cookie = %+v", c)
	}
	if !c.Expires.Equal(g.exch.exp) {
		t.Errorf("Expires = %v, want the session expiry %v", c.Expires, g.exch.exp)
	}

	w = g.do("GET", "crm."+testDomain, "/__foyr/callback?code=abc&return=%2F%2Fevil.com")
	if w.Header().Get("Location") != "/" {
		t.Errorf("open redirect: Location = %q, want /", w.Header().Get("Location"))
	}
}

func TestCallbackFailureShowsExpiredPage(t *testing.T) {
	g := newAuthRig(t)
	g.exch.err = errors.New("expired")
	w := g.do("GET", "crm."+testDomain, "/__foyr/callback?code=bad")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "This sign-in link has expired") ||
		!strings.Contains(w.Body.String(), testAuthURL+"/login?app=crm") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Error("a cookie was set after a failed exchange")
	}
	if w := g.do("GET", "crm."+testDomain, "/__foyr/callback"); w.Code != http.StatusBadRequest {
		t.Errorf("no code: %d, want 400", w.Code)
	}
}

func TestLogoutNeedsSameOrigin(t *testing.T) {
	g := newAuthRig(t)
	tok := g.session("crm", 7, "s1")
	for _, origin := range []string{"https://evil.com", ""} {
		opts := []reqOpt{withCookie(testCookie + "=" + tok)}
		if origin != "" {
			opts = append(opts, withHeader("Origin", origin))
		}
		if w := g.do("POST", "crm."+testDomain, "/__foyr/logout", opts...); w.Code != http.StatusForbidden {
			t.Errorf("Origin %q: %d, want 403", origin, w.Code)
		}
	}
	if len(g.exch.logouts) != 0 {
		t.Fatal("a cross-site logout reached foyrd")
	}
	w := g.do("POST", "crm."+testDomain, "/__foyr/logout", withCookie(testCookie+"="+tok), withHeader("Origin", "https://crm."+testDomain))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("logout: %d %q", w.Code, w.Header().Get("Location"))
	}
	if len(g.exch.logouts) != 1 || g.exch.logouts[0] != "s1" {
		t.Errorf("logouts = %v, want [s1]", g.exch.logouts)
	}
	cs := w.Result().Cookies()
	if len(cs) != 1 || cs[0].Name != testCookie || cs[0].MaxAge >= 0 || cs[0].Value != "" {
		t.Errorf("cookie not cleared: %v", cs)
	}
}

func TestAuthOff(t *testing.T) {
	// No Auth and no Access: private-looking paths pass straight through, as before login existed.
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(echo{Headers: r.Header, Cookie: r.Header.Get("Cookie")})
	}))
	defer app.Close()
	tbl := &Table{}
	tbl.Set(map[string]string{"crm": strings.TrimPrefix(app.URL, "http://")})
	h := obs.RequestID(obs.NewLogger(io.Discard), NewHandler(testDomain, tbl))
	w := do(h, "crm."+testDomain, "/", map[string]string{"Accept": "text/html"})
	if w.Code != http.StatusOK {
		t.Fatalf("auth off: %d, want 200", w.Code)
	}
	if w := do(h, "crm."+testDomain, "/__foyr/callback?code=x", nil); w.Code != http.StatusOK {
		t.Errorf("auth off: /__foyr/callback = %d, want it passed to the app (200)", w.Code)
	}
}

func TestAccessPollerKeepsTheLastGoodSnapshot(t *testing.T) {
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, `{"apps":[{"slug":"crm","mode":"private","members":{"7":"admin"}}],"revoked":["s1"]}`)
	}))
	defer srv.Close()
	a := &Access{}
	p := &AccessPoller{URL: srv.URL, Token: "tok", Access: a, Log: obs.NewLogger(io.Discard)}
	if a.Ready() {
		t.Fatal("ready before any fetch")
	}
	snap, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a.Set(snap)
	app, ok := a.App("crm")
	if !a.Ready() || !ok || app.Mode != "private" || app.Members[7] != "admin" || !a.Revoked("s1") || a.Revoked("s2") {
		t.Fatalf("snapshot = %+v ok=%v", app, ok)
	}
	fail = true
	if _, err := p.Fetch(context.Background()); err == nil {
		t.Error("a 502 was accepted")
	}
	if !a.Ready() {
		t.Error("the snapshot was dropped")
	}
}

func TestHTTPExchanger(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case "/internal/auth/exchange":
			if in["code"] != "good" || in["slug"] != "crm" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"token":"TOK","expires_at":"2026-11-01T10:00:00Z"}`)
		case "/internal/auth/logout":
			if in["sid"] != "s1" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	x := &HTTPExchanger{BaseURL: srv.URL, Token: "tok"}
	tok, exp, err := x.Exchange(context.Background(), "good", "crm")
	if err != nil || tok != "TOK" || !exp.Equal(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("Exchange = %q %v %v", tok, exp, err)
	}
	if _, _, err := x.Exchange(context.Background(), "bad", "crm"); err == nil {
		t.Error("a 400 was accepted")
	}
	if err := x.Logout(context.Background(), "s1"); err != nil {
		t.Errorf("Logout: %v", err)
	}
	if err := x.Logout(context.Background(), "other"); err == nil {
		t.Error("a 400 on logout was accepted")
	}
}

func tokenKid(t *testing.T, tok string) string {
	t.Helper()
	seg, _, _ := strings.Cut(tok, ".")
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		t.Fatal(err)
	}
	var h struct{ Kid string }
	_ = json.Unmarshal(raw, &h)
	return h.Kid
}

func TestAssertionAccompaniesIdentity(t *testing.T) {
	g := newAuthRig(t)
	e := seen(t, g.do("GET", "crm."+testDomain, "/", withCookie(testCookie+"="+g.session("crm", 7, "s1"))))
	for _, k := range []string{"X-Foyr-User-Id", "X-Foyr-Email", "X-Foyr-Role", "X-Foyr-Assertion"} {
		if e.Headers.Get(k) == "" {
			t.Errorf("missing %s", k)
		}
	}
	if e.Headers.Get("X-Foyr-User-Id") != testPID {
		t.Errorf("user id header %q", e.Headers.Get("X-Foyr-User-Id"))
	}
	tok := e.Headers.Get("X-Foyr-Assertion")
	c, err := authn.VerifyAssertion(authn.KeySet([]*ecdsa.PublicKey{g.apub}), tok, testAuthURL, "crm", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if c.Sub != testPID || c.Email != "ana@example.com" || c.Role != "admin" || c.Exp-c.Iat != 60 {
		t.Errorf("claims = %+v", c)
	}
	if tokenKid(t, tok) != authn.KeyID(g.apub) {
		t.Error("kid does not match the key")
	}
}

func TestViewerOfPublicAppGetsAViewerAssertion(t *testing.T) {
	g := newAuthRig(t)
	e := seen(t, g.do("GET", "blog."+testDomain, "/", withCookie(testCookie+"="+g.session("blog", 42, "s9"))))
	c, err := authn.VerifyAssertion(authn.KeySet([]*ecdsa.PublicKey{g.apub}), e.Headers.Get("X-Foyr-Assertion"), testAuthURL, "blog", time.Now())
	if err != nil || c.Role != "viewer" || c.Sub != testPID {
		t.Fatalf("claims = %+v err = %v", c, err)
	}
}

func TestAnonymousVisitorGetsNoIdentityHeaders(t *testing.T) {
	g := newAuthRig(t)
	e := seen(t, g.do("GET", "blog."+testDomain, "/"))
	for _, k := range []string{"X-Foyr-User-Id", "X-Foyr-Email", "X-Foyr-Role", "X-Foyr-Assertion"} {
		if _, ok := e.Headers[k]; ok {
			t.Errorf("anonymous visitor got %s", k)
		}
	}
}

func TestVisitorSentAssertionIsDeleted(t *testing.T) {
	g := newAuthRig(t)
	e := seen(t, g.do("GET", "blog."+testDomain, "/", withHeader("X-Foyr-Assertion", "forged")))
	if _, ok := e.Headers["X-Foyr-Assertion"]; ok {
		t.Error("a forged assertion reached the app (anonymous)")
	}
	e = seen(t, g.do("GET", "crm."+testDomain, "/", withHeader("X-Foyr-Assertion", "forged"),
		withCookie(testCookie+"="+g.session("crm", 7, "s1"))))
	vals := e.Headers.Values("X-Foyr-Assertion")
	if len(vals) != 1 || vals[0] == "forged" {
		t.Fatalf("assertion headers = %v", vals)
	}
	if _, err := authn.VerifyAssertion(authn.KeySet([]*ecdsa.PublicKey{g.apub}), vals[0], testAuthURL, "crm", time.Now()); err != nil {
		t.Errorf("the genuine assertion does not verify: %v", err)
	}
}

func TestSessionWithoutPublicIDIsSignedOut(t *testing.T) {
	g := newAuthRig(t)
	old := g.sessionFor("crm", 7, "s1", "")
	w := g.do("GET", "crm."+testDomain, "/", withHeader("Accept", "text/html"), withCookie(testCookie+"="+old))
	if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), testAuthURL+"/login?app=crm") {
		t.Fatalf("browser: %d %q", w.Code, w.Header().Get("Location"))
	}
	w = g.do("GET", "crm."+testDomain, "/api/x", withCookie(testCookie+"="+old))
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "login required") {
		t.Fatalf("api: %d %s", w.Code, w.Body)
	}
	if g.hits != 0 {
		t.Fatal("the app was reached with a session lacking a public id")
	}
	if w := g.do("GET", "crm."+testDomain, "/", withCookie(testCookie+"="+g.session("crm", 7, "s2"))); w.Code != http.StatusOK {
		t.Errorf("fresh session: %d", w.Code)
	}
}

func TestSigningFailureAnswers500AndForwardsNothing(t *testing.T) {
	g := newAuthRig(t)
	g.auth.AssertionKey = nil
	w := g.do("GET", "crm."+testDomain, "/", withCookie(testCookie+"="+g.session("crm", 7, "s1")))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", w.Code)
	}
	if g.hits != 0 {
		t.Error("the request was forwarded")
	}
}

func TestCallbackRefusesATokenWithoutPublicID(t *testing.T) {
	g := newAuthRig(t)
	g.exch.token = g.sessionFor("crm", 7, "s1", "")
	w := g.do("GET", "crm."+testDomain, "/__foyr/callback?code=abc")
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), g.exch.token) {
		t.Errorf("status %d, want 502 and no token in the page", w.Code)
	}
	if len(w.Result().Cookies()) != 0 || w.Header().Get("Set-Cookie") != "" {
		t.Error("a cookie was set")
	}
	if g.hits != 0 {
		t.Error("the app was hit")
	}
}

func TestCallbackRefusesABadToken(t *testing.T) {
	g := newAuthRig(t)
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	for name, tok := range map[string]string{
		"garbage":   "v1.x.y",
		"wrong key": authn.Sign(otherPriv, authn.Claims{SID: "s", UserID: 7, PublicID: testPID, Slug: "crm", Exp: time.Now().Add(time.Hour).Unix()}),
	} {
		g.exch.token = tok
		w := g.do("GET", "crm."+testDomain, "/__foyr/callback?code=abc")
		if w.Code != http.StatusBadGateway || len(w.Result().Cookies()) != 0 {
			t.Errorf("%s: status %d cookies %v, want 502 and none", name, w.Code, w.Result().Cookies())
		}
	}
	if g.hits != 0 {
		t.Error("the app was hit")
	}
}

func TestConnectionHeaderCannotStripIdentity(t *testing.T) {
	g := newAuthRig(t)
	conns := []string{"X-Foyr-Assertion", "X-Foyr-User-Id", "X-Foyr-Role, X-Foyr-Email",
		"X-Foyr-Assertion, X-Foyr-User-Id, X-Foyr-Role, X-Foyr-Email, close"}
	for _, c := range conns {
		e := seen(t, g.do("GET", "crm."+testDomain, "/", withHeader("Connection", c), withCookie(testCookie+"="+g.session("crm", 7, "s1"))))
		if e.Headers.Get("X-Foyr-User-Id") != testPID || e.Headers.Get("X-Foyr-Email") != "ana@example.com" || e.Headers.Get("X-Foyr-Role") != "admin" {
			t.Errorf("Connection %q: identity headers = %v", c, e.Headers)
		}
		if _, err := authn.VerifyAssertion(authn.KeySet([]*ecdsa.PublicKey{g.apub}), e.Headers.Get("X-Foyr-Assertion"), testAuthURL, "crm", time.Now()); err != nil {
			t.Errorf("Connection %q: assertion: %v", c, err)
		}
		an := seen(t, g.do("GET", "blog."+testDomain, "/", withHeader("Connection", c)))
		for _, k := range []string{"X-Foyr-User-Id", "X-Foyr-Email", "X-Foyr-Role", "X-Foyr-Assertion"} {
			if _, ok := an.Headers[k]; ok {
				t.Errorf("Connection %q: anonymous got %s", c, k)
			}
		}
	}
}

// Owners and admins of an app's account arrive in the snapshot like explicit members (ADR-039).
func TestAccountOwnerInTheSnapshotIsAdmittedUntilRemoved(t *testing.T) {
	g := newAuthRig(t)
	g.access.Set(authn.AccessSnapshot{Apps: []authn.AppAccess{{Slug: "crm", Mode: "private", Members: map[int64]string{7: "owner"}}}})
	tok := g.session("crm", 7, "s9")
	e := seen(t, g.do("GET", "crm."+testDomain, "/", withHeader("Accept", "text/html"), withCookie(testCookie+"="+tok)))
	if e.Headers.Get("X-Foyr-Role") != "owner" {
		t.Errorf("role = %q, want owner", e.Headers.Get("X-Foyr-Role"))
	}
	g.access.Set(authn.AccessSnapshot{Apps: []authn.AppAccess{{Slug: "crm", Mode: "private", Members: map[int64]string{}}}})
	if w := g.do("GET", "crm."+testDomain, "/", withHeader("Accept", "text/html"), withCookie(testCookie+"="+tok)); w.Code != http.StatusForbidden {
		t.Errorf("after leaving the account: %d, want 403", w.Code)
	}
}
