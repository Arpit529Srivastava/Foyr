package login

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/mail"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

const (
	authHost = "auth.foyr.test"
	authURL  = "https://auth.foyr.test"
)

// fakeStore lets ana@example.com into crm and counts what the handler asks for.
type fakeStore struct {
	mu        sync.Mutex
	started   int
	finished  int
	tokens    map[string]string // raw token to slug, until used
	nextToken int
}

func (f *fakeStore) StartLogin(_ context.Context, slug, email string, _ time.Time) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started++
	if slug != "crm" || email != "ana@example.com" {
		return "", false, nil
	}
	f.nextToken++
	raw := "tok" + string(rune('a'+f.nextToken))
	if f.tokens == nil {
		f.tokens = map[string]string{}
	}
	f.tokens[raw] = slug
	return raw, true, nil
}

func (f *fakeStore) FinishLogin(_ context.Context, raw string, _ time.Time) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	slug, ok := f.tokens[raw]
	if !ok {
		return "", "", store.ErrInvalidToken
	}
	delete(f.tokens, raw)
	f.finished++
	return "code123", slug, nil
}

type recorder struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (r *recorder) Send(_ context.Context, m mail.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, m)
	return nil
}

type rig struct {
	h    *Handler
	mux  *http.ServeMux
	fs   *fakeStore
	rec  *recorder
	now  time.Time
	addr string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	g := &rig{fs: &fakeStore{}, rec: &recorder{}, now: time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC), addr: "203.0.113.5"}
	g.h = &Handler{Store: g.fs, Mail: g.rec, AuthURL: authURL, AppsDomain: "apps.test", Now: func() time.Time { return g.now }}
	g.mux = http.NewServeMux()
	g.h.Register(g.mux)
	return g
}

func (g *rig) do(method, target, host, origin string, form url.Values) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, "https://"+host+target, body)
	r.Host = host
	r.RemoteAddr = g.addr + ":4000"
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	g.mux.ServeHTTP(w, r)
	return w
}

func (g *rig) get(target string) *httptest.ResponseRecorder {
	return g.do("GET", target, authHost, "", nil)
}

func (g *rig) post(target string, form url.Values) *httptest.ResponseRecorder {
	return g.do("POST", target, authHost, authURL, form)
}

func (g *rig) sent() []mail.Message {
	g.h.Wait()
	g.rec.mu.Lock()
	defer g.rec.mu.Unlock()
	return append([]mail.Message(nil), g.rec.sent...)
}

func loginForm(app, email string) url.Values {
	return url.Values{"app": {app}, "email": {email}, "return": {"/reports"}}
}

func TestLoginDoesNotRevealMembers(t *testing.T) {
	g := newRig(t)
	member := g.post("/login", loginForm("crm", "ana@example.com"))
	stranger := g.post("/login", loginForm("crm", "eve@example.com"))
	unknown := g.post("/login", loginForm("no-such-app", "ana@example.com"))

	for name, w := range map[string]*httptest.ResponseRecorder{"member": member, "stranger": stranger, "unknown app": unknown} {
		if w.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200", name, w.Code)
		}
	}
	if member.Body.String() != stranger.Body.String() || member.Body.String() != unknown.Body.String() {
		t.Error("the three answers differ, so they reveal who is a member")
	}
	sent := g.sent()
	if len(sent) != 1 || sent[0].To != "ana@example.com" || sent[0].Subject != "Sign in to crm" {
		t.Fatalf("sent = %+v, want one email to ana", sent)
	}
	if !strings.Contains(sent[0].Text, authURL+"/login/verify?") || !strings.Contains(sent[0].Text, "once, for 15 minutes") {
		t.Errorf("email text = %q", sent[0].Text)
	}
	link := regexpLink(t, sent[0].Text)
	if link.Query().Get("app") != "crm" || link.Query().Get("return") != "/reports" || link.Query().Get("t") == "" {
		t.Errorf("link %s lacks app, return or token", link)
	}
}

func regexpLink(t *testing.T, text string) *url.URL {
	t.Helper()
	for _, f := range strings.Fields(text) {
		if strings.HasPrefix(f, authURL+"/login/verify?") {
			u, err := url.Parse(f)
			if err != nil {
				t.Fatal(err)
			}
			return u
		}
	}
	t.Fatalf("no verify link in %q", text)
	return nil
}

func TestLoginFormAndBadEmail(t *testing.T) {
	g := newRig(t)
	w := g.get("/login?app=crm&return=%2Freports")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `name="email"`) || !strings.Contains(w.Body.String(), "crm") {
		t.Fatalf("form: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `value="/reports"`) {
		t.Error("return path not carried in the form")
	}
	if strings.Contains(g.get("/login?app=%3Cscript%3E").Body.String(), "<script>") {
		t.Error("app parameter is reflected unescaped")
	}
	bad := g.post("/login", loginForm("crm", "not an email"))
	if bad.Code != http.StatusBadRequest || len(g.sent()) != 0 {
		t.Errorf("bad email: status %d, %d emails", bad.Code, len(g.sent()))
	}
}

func TestVerifyGetDoesNotUseToken(t *testing.T) {
	g := newRig(t)
	g.post("/login", loginForm("crm", "ana@example.com"))
	link := regexpLink(t, g.sent()[0].Text)
	target := "/login/verify?" + link.RawQuery

	for range 2 {
		w := g.get(target)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Continue to crm") {
			t.Fatalf("verify page: %d %s", w.Code, w.Body.String())
		}
	}
	if g.fs.finished != 0 {
		t.Fatalf("GET spent the token (%d finishes); email scanners would burn every link", g.fs.finished)
	}
	w := g.post("/login/verify", url.Values{"t": {link.Query().Get("t")}, "return": {"/reports"}})
	if w.Code != http.StatusSeeOther || g.fs.finished != 1 {
		t.Fatalf("POST: status %d, finishes %d; want 303 and 1", w.Code, g.fs.finished)
	}
}

func TestVerifyRedirect(t *testing.T) {
	g := newRig(t)
	cases := []struct{ ret, want string }{
		{"/reports", "https://crm.apps.test/__foyr/callback?code=code123&return=%2Freports"},
		{"//evil.com", "https://crm.apps.test/__foyr/callback?code=code123&return=%2F"},
		{"", "https://crm.apps.test/__foyr/callback?code=code123&return=%2F"},
	}
	for _, c := range cases {
		g.post("/login", loginForm("crm", "ana@example.com"))
		sent := g.sent()
		tok := regexpLink(t, sent[len(sent)-1].Text).Query().Get("t")
		w := g.post("/login/verify", url.Values{"t": {tok}, "return": {c.ret}})
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != c.want {
			t.Errorf("return %q: %d %s, want 303 %s", c.ret, w.Code, w.Header().Get("Location"), c.want)
		}
	}
}

func TestVerifyPageAllowsTheAppHostToReceiveTheForm(t *testing.T) {
	g := newRig(t)
	csp := g.get("/login/verify?t=x&app=crm").Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "form-action 'self' https://*.apps.test") {
		t.Errorf("csp = %q; Chrome blocks the redirect to the app host otherwise", csp)
	}
	if strings.Contains(g.get("/login").Header().Get("Content-Security-Policy"), "apps.test") {
		t.Error("only the verify page may post towards the apps domain")
	}
}

func TestVerifyInvalidToken(t *testing.T) {
	g := newRig(t)
	w := g.post("/login/verify", url.Values{"t": {"unknown"}})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "expired or was already used") ||
		!strings.Contains(w.Body.String(), "/login") {
		t.Errorf("invalid token: %d %s", w.Code, w.Body.String())
	}
	if w := g.post("/login/verify", url.Values{}); w.Code != http.StatusBadRequest {
		t.Errorf("missing token: %d", w.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	g := newRig(t)
	first := g.post("/login", loginForm("crm", "ana@example.com"))
	g.post("/login", loginForm("crm", " ANA@example.com "))
	g.post("/login", loginForm("crm", "ana@example.com"))
	if n := len(g.sent()); n != 3 {
		t.Fatalf("%d emails after 3 requests, want 3", n)
	}
	fourth := g.post("/login", loginForm("crm", "ana@example.com"))
	if n := len(g.sent()); n != 3 {
		t.Errorf("%d emails after the fourth request, want still 3", n)
	}
	if fourth.Code != http.StatusOK || fourth.Body.String() != first.Body.String() {
		t.Error("the limited answer differs from the normal one")
	}
	g.now = g.now.Add(16 * time.Minute)
	g.post("/login", loginForm("crm", "ana@example.com"))
	if n := len(g.sent()); n != 4 {
		t.Errorf("%d emails after the window passed, want 4", n)
	}
}

func TestLoginRateLimitPerIP(t *testing.T) {
	g := newRig(t)
	for i := range 11 {
		g.post("/login", loginForm("crm", "bob"+strings.Repeat("x", i)+"@example.com"))
	}
	g.post("/login", loginForm("crm", "ana@example.com"))
	if n := len(g.sent()); n != 0 {
		t.Errorf("%d emails sent, want none (those addresses are not members)", n)
	}
	if g.fs.started != 10 {
		t.Errorf("store asked %d times, want 10: posts past the IP limit must not reach it", g.fs.started)
	}
	g.addr = "203.0.113.99"
	g.post("/login", loginForm("crm", "ana@example.com"))
	if n := len(g.sent()); n != 1 {
		t.Errorf("another address got %d emails, want 1", n)
	}
}

func TestLoginOtherHost(t *testing.T) {
	g := newRig(t)
	for _, c := range []struct{ method, path string }{{"GET", "/login"}, {"POST", "/login"}, {"GET", "/login/verify?t=x"}, {"POST", "/login/verify"}} {
		w := g.do(c.method, c.path, "api.foyr.test", "https://api.foyr.test", url.Values{})
		if w.Code != http.StatusNotFound {
			t.Errorf("%s %s on another host: %d, want 404", c.method, c.path, w.Code)
		}
	}
}

func TestLoginCrossOrigin(t *testing.T) {
	g := newRig(t)
	for _, path := range []string{"/login", "/login/verify"} {
		w := g.do("POST", path, authHost, "https://evil.com", loginForm("crm", "ana@example.com"))
		if w.Code != http.StatusForbidden {
			t.Errorf("POST %s with a foreign Origin: %d, want 403", path, w.Code)
		}
		if w := g.do("POST", path, authHost, "", loginForm("crm", "ana@example.com")); w.Code != http.StatusForbidden {
			t.Errorf("POST %s with no Origin: %d, want 403", path, w.Code)
		}
	}
	if len(g.sent()) != 0 || g.fs.started != 0 {
		t.Error("a cross-site post reached the store")
	}
}

func TestPagesSendSecurityHeaders(t *testing.T) {
	g := newRig(t)
	g.post("/login", loginForm("crm", "ana@example.com"))
	tok := regexpLink(t, g.sent()[0].Text).Query().Get("t")
	pages := map[string]*httptest.ResponseRecorder{
		"form":    g.get("/login?app=crm"),
		"sent":    g.post("/login", loginForm("crm", "eve@example.com")),
		"verify":  g.get("/login/verify?t=" + tok + "&app=crm"),
		"expired": g.post("/login/verify", url.Values{"t": {"nope"}}),
	}
	nonces := map[string]bool{}
	for name, w := range pages {
		h := w.Header()
		csp := h.Get("Content-Security-Policy")
		i := strings.Index(csp, "'nonce-")
		if i < 0 {
			t.Errorf("%s: CSP has no nonce: %q", name, csp)
		} else {
			nonces[csp[i:i+len("'nonce-")+22]] = true
		}
		for k, want := range map[string]string{"X-Frame-Options": "DENY", "Cache-Control": "no-store", "Referrer-Policy": "same-origin"} {
			if h.Get(k) != want {
				t.Errorf("%s: %s = %q, want %q", name, k, h.Get(k), want)
			}
		}
		if !strings.HasPrefix(h.Get("Content-Type"), "text/html") {
			t.Errorf("%s: Content-Type %q", name, h.Get("Content-Type"))
		}
	}
	if len(nonces) != len(pages) {
		t.Error("nonce repeats between pages")
	}
}
