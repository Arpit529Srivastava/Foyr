package dashboard

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
	dashHost   = "app.foyr.test"
	dashOrigin = "https://app.foyr.test"
)

// fakeAuth stands in for the store's dashboard login.
type fakeAuth struct {
	mu       sync.Mutex
	linked   map[string]bool                // emails that may sign in
	tokens   map[string]string              // raw token to email
	sessions map[string]store.DashboardUser // cookie value to user
	revoked  []string
	n        int
	ssoCalls int // IssueSSOCode calls
}

func newAuth() *fakeAuth {
	return &fakeAuth{linked: map[string]bool{"ana@acme.test": true}, tokens: map[string]string{}, sessions: map[string]store.DashboardUser{
		"ana-cookie": {ID: 7, Email: "ana@acme.test", Accounts: []store.AccountAccess{{Installation: store.Installation{ID: 1, Login: "acme"}, Role: "owner"}}},
		"mia-cookie": {ID: 8, Email: "mia@acme.test", Accounts: []store.AccountAccess{{Installation: store.Installation{ID: 1, Login: "acme"}, Role: "member"}}},
		"zed-cookie": {ID: 9, Email: "zed@other.test", Accounts: []store.AccountAccess{{Installation: store.Installation{ID: 2, Login: "other"}, Role: "owner"}}},
	}}
}

func (a *fakeAuth) StartDashboardLogin(_ context.Context, email string, _ time.Time) (string, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.linked[email] {
		return "", false, nil
	}
	a.n++
	raw := "tok" + string(rune('a'+a.n))
	a.tokens[raw] = email
	return raw, true, nil
}

func (a *fakeAuth) FinishDashboardLogin(_ context.Context, raw string, now time.Time) (store.DashboardSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	email, ok := a.tokens[raw]
	if !ok {
		return store.DashboardSession{}, store.ErrInvalidToken
	}
	delete(a.tokens, raw)
	a.sessions["new-cookie"] = store.DashboardUser{ID: 7, Email: email, Accounts: a.sessions["ana-cookie"].Accounts}
	return store.DashboardSession{Token: "new-cookie", ExpiresAt: now.Add(store.DashboardSessionTTL)}, nil
}

func (a *fakeAuth) DashboardUserBySession(_ context.Context, raw string, _ time.Time) (store.DashboardUser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	u, ok := a.sessions[raw]
	if !ok {
		return store.DashboardUser{}, store.ErrInvalidToken
	}
	return u, nil
}

func (a *fakeAuth) RevokeDashboardSession(_ context.Context, raw string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revoked = append(a.revoked, raw)
	delete(a.sessions, raw)
	return nil
}

func (a *fakeAuth) has(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.tokens[token]
	return ok
}

type loginRig struct {
	h    *Handler
	mux  http.Handler
	auth *fakeAuth
	box  *outbox
}

func newRig(founder bool) *loginRig {
	r := &loginRig{auth: newAuth(), box: &outbox{}}
	r.h = &Handler{Data: fake{m: newMem()}, Auth: r.auth, DashURL: dashOrigin, AuthURL: "https://auth.foyr.test", AppsScheme: "https",
		SignIn: "/founder", AppsDomain: "apps.test",
		Mail: r.box, SiteURL: "https://foyr.test", Now: func() time.Time { return now }}
	if founder {
		r.h.Who = func(req *http.Request) (Person, bool) {
			c, err := req.Cookie("founder")
			return Person{Founder: true}, err == nil && c.Value == "yes"
		}
	}
	mux := http.NewServeMux()
	r.h.Register(mux)
	r.mux = mux
	return r
}

type opt func(*http.Request)

func cookie(name, v string) opt {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: name, Value: v}) }
}
func host(h string) opt   { return func(r *http.Request) { r.Host = h } }
func origin(o string) opt { return func(r *http.Request) { r.Header.Set("Origin", o) } }
func ip(a string) opt     { return func(r *http.Request) { r.RemoteAddr = a + ":4000" } }

// do sends a request; parts are a url.Values form and any number of opts, in any order.
func (g *loginRig) do(method, path string, parts ...any) *httptest.ResponseRecorder {
	form := url.Values{}
	var opts []opt
	for _, p := range parts {
		switch v := p.(type) {
		case url.Values:
			form = v
		case opt:
			opts = append(opts, v)
		case func(*http.Request):
			opts = append(opts, v)
		case nil:
		default:
			panic("do: unsupported part")
		}
	}
	r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	r.Host = dashHost
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", dashOrigin)
	}
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	g.mux.ServeHTTP(w, r)
	g.h.Wait()
	return w
}

func TestSignInPagesOnlyOnTheDashboardHost(t *testing.T) {
	g := newRig(false)
	if w := g.do("GET", "/dashboard/login"); w.Code != 200 || !strings.Contains(w.Body.String(), `name="email"`) {
		t.Fatalf("%d\n%s", w.Code, w.Body.String())
	}
	for _, p := range []string{"/dashboard/login", "/dashboard/login/verify?t=x"} {
		if w := g.do("GET", p, nil, host("api.foyr.test")); w.Code != http.StatusNotFound {
			t.Errorf("GET %s on another host: %d, want 404", p, w.Code)
		}
	}
	if w := g.do("POST", "/dashboard/login", url.Values{"email": {"ana@acme.test"}}, host("evil.test")); w.Code != http.StatusNotFound {
		t.Errorf("POST on another host: %d", w.Code)
	}
	off := &Handler{Data: fake{m: newMem()}, SignIn: "/founder"}
	mux := http.NewServeMux()
	off.Register(mux)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/dashboard/login", nil)
	r.Host = dashHost
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("sign-in with no Auth configured: %d, want 404", w.Code)
	}
	if w := g.do("GET", "/"); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard" {
		t.Errorf("root: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := g.do("GET", "/", host("api.foyr.test")); w.Code != http.StatusNotFound {
		t.Errorf("root on another host: %d, want 404", w.Code)
	}
}

func TestLinkedEmailGetsALinkAndStrangersGetTheSameAnswer(t *testing.T) {
	g := newRig(false)
	member := g.do("POST", "/dashboard/login", url.Values{"email": {" Ana@Acme.test "}})
	stranger := g.do("POST", "/dashboard/login", url.Values{"email": {"nobody@acme.test"}})
	if member.Code != 200 || member.Body.String() != stranger.Body.String() || member.Code != stranger.Code {
		t.Fatalf("answers differ:\n%d %s\n%d %s", member.Code, member.Body.String(), stranger.Code, stranger.Body.String())
	}
	if len(g.box.sent) != 1 || g.box.sent[0].To != "ana@acme.test" {
		t.Fatalf("sent = %+v, want one email to ana only", g.box.sent)
	}
	link := g.box.sent[0].Text
	if !strings.Contains(link, dashOrigin+"/dashboard/login/verify?t=") {
		t.Errorf("link is not on the dashboard host: %q", link)
	}
	if w := g.do("POST", "/dashboard/login", url.Values{"email": {"not an email"}}); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "valid email") {
		t.Errorf("bad email: %d", w.Code)
	}
}

func TestLoginIsRateLimited(t *testing.T) {
	g := newRig(false)
	for i := range 6 {
		g.do("POST", "/dashboard/login", url.Values{"email": {"ana@acme.test"}}, ip("203.0.113.5"))
		_ = i
	}
	if len(g.box.sent) != 3 {
		t.Fatalf("%d emails for one address, want 3 in the window", len(g.box.sent))
	}
	g2 := newRig(false)
	for i := range 14 {
		g2.do("POST", "/dashboard/login", url.Values{"email": {"nobody" + string(rune('a'+i)) + "@acme.test"}}, ip("203.0.113.9"))
	}
	g2.do("POST", "/dashboard/login", url.Values{"email": {"ana@acme.test"}}, ip("203.0.113.9"))
	if len(g2.box.sent) != 0 {
		t.Errorf("an address past the IP limit still got an email")
	}
	if w := g2.do("POST", "/dashboard/login", url.Values{"email": {"ana@acme.test"}}, ip("198.51.100.7")); w.Code != 200 || len(g2.box.sent) != 1 {
		t.Errorf("another IP is blocked: %d, %d emails", w.Code, len(g2.box.sent))
	}
}

func TestPostsNeedTheSameOrigin(t *testing.T) {
	g := newRig(false)
	for _, p := range []string{"/dashboard/login", "/dashboard/login/verify", "/dashboard/logout"} {
		for _, o := range []string{"", "null", "https://evil.test"} {
			r := httptest.NewRequest("POST", p, strings.NewReader("email=ana%40acme.test&t=x"))
			r.Host = dashHost
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if o != "" {
				r.Header.Set("Origin", o)
			}
			w := httptest.NewRecorder()
			g.mux.ServeHTTP(w, r)
			if w.Code != http.StatusForbidden {
				t.Errorf("POST %s origin %q: %d, want 403", p, o, w.Code)
			}
		}
	}
	if len(g.box.sent) != 0 {
		t.Error("a cross-site post sent an email")
	}
}

// A mail scanner opening the link must not use up the token.
func TestOpeningTheLinkDoesNotSpendTheToken(t *testing.T) {
	g := newRig(false)
	g.auth.tokens["tok1"] = "ana@acme.test"
	w := g.do("GET", "/dashboard/login/verify?t=tok1")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Continue") || !strings.Contains(w.Body.String(), `value="tok1"`) {
		t.Fatalf("%d\n%s", w.Code, w.Body.String())
	}
	if !g.auth.has("tok1") || len(w.Result().Cookies()) != 0 {
		t.Error("GET used the token or set a cookie")
	}
	if w := g.do("GET", "/dashboard/login/verify"); w.Code != http.StatusBadRequest {
		t.Errorf("no token: %d, want 400", w.Code)
	}
}

func TestContinueSignsInOnce(t *testing.T) {
	g := newRig(false)
	g.auth.tokens["tok1"] = "ana@acme.test"
	w := g.do("POST", "/dashboard/login/verify", url.Values{"t": {"tok1"}})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard" {
		t.Fatalf("%d %q", w.Code, w.Header().Get("Location"))
	}
	cs := w.Result().Cookies()
	if len(cs) != 1 {
		t.Fatalf("cookies = %+v", cs)
	}
	c := cs[0]
	if c.Name != "__Host-foyr_dash" || c.Value != "new-cookie" || !c.HttpOnly || !c.Secure || c.Path != "/" || c.Domain != "" ||
		c.SameSite != http.SameSiteLaxMode || c.MaxAge != int(store.DashboardSessionTTL.Seconds()) {
		t.Errorf("cookie = %+v", c)
	}
	again := g.do("POST", "/dashboard/login/verify", url.Values{"t": {"tok1"}})
	if again.Code != http.StatusBadRequest || len(again.Result().Cookies()) != 0 {
		t.Errorf("second use: %d, cookies %v", again.Code, again.Result().Cookies())
	}
	if w := g.do("POST", "/dashboard/login/verify", url.Values{"t": {"nope"}}); w.Code != http.StatusBadRequest {
		t.Errorf("bad token: %d", w.Code)
	}
}

func TestSignedInCustomerSeesOnlyTheirAccount(t *testing.T) {
	g := newRig(false)
	c := cookie("__Host-foyr_dash", "ana-cookie")
	if w := g.do("GET", "/dashboard", c); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard/acme" {
		t.Fatalf("start: %d %q", w.Code, w.Header().Get("Location"))
	}
	body := g.do("GET", "/dashboard/acme", c).Body.String()
	if !strings.Contains(body, "https://crm.apps.test") || strings.Contains(body, "other-co") || strings.Contains(body, "as the founder") {
		t.Fatalf("account page\n%s", body)
	}
	if !strings.Contains(body, `action="/dashboard/logout"`) {
		t.Error("no sign-out button")
	}
	if w := g.do("GET", "/dashboard/other-co", c); w.Code != http.StatusNotFound {
		t.Errorf("other account: %d", w.Code)
	}
	if w := g.do("GET", "/dashboard/login", c); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard" {
		t.Errorf("sign-in page while signed in: %d %q", w.Code, w.Header().Get("Location"))
	}
}

func TestSignedOutVisitorsAreSentToSignIn(t *testing.T) {
	g := newRig(false)
	for _, c := range []opt{func(*http.Request) {}, cookie("__Host-foyr_dash", "forged"), cookie("founder", "yes"), cookie("__Host-foyr_founder", "x")} {
		if w := g.do("GET", "/dashboard/acme", c); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard/login" {
			t.Errorf("%d %q", w.Code, w.Header().Get("Location"))
		}
	}
	// On another host the founder's own sign-in page is still the way in.
	if w := g.do("GET", "/dashboard/acme", host("api.foyr.test")); w.Header().Get("Location") != "/founder" {
		t.Errorf("other host: %q", w.Header().Get("Location"))
	}
}

func TestFounderIsUnaffected(t *testing.T) {
	g := newRig(true)
	w := g.do("GET", "/dashboard/other-co", cookie("founder", "yes"))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "as the founder") || strings.Contains(w.Body.String(), `action="/dashboard/logout"`) {
		t.Fatalf("%d", w.Code)
	}
}

func TestSignOut(t *testing.T) {
	g := newRig(false)
	c := cookie("__Host-foyr_dash", "ana-cookie")
	w := g.do("POST", "/dashboard/logout", c)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard/login" {
		t.Fatalf("%d %q", w.Code, w.Header().Get("Location"))
	}
	if len(g.auth.revoked) != 1 || g.auth.revoked[0] != "ana-cookie" {
		t.Errorf("revoked = %v", g.auth.revoked)
	}
	cs := w.Result().Cookies()
	if len(cs) != 1 || cs[0].Name != "__Host-foyr_dash" || cs[0].MaxAge >= 0 {
		t.Errorf("cookie not cleared: %+v", cs)
	}
	if w := g.do("GET", "/dashboard/acme", c); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard/login" {
		t.Errorf("after sign-out: %d %q", w.Code, w.Header().Get("Location"))
	}
	// Signing out without a session is harmless.
	if w := g.do("POST", "/dashboard/logout", nil); w.Code != http.StatusSeeOther {
		t.Errorf("no session: %d", w.Code)
	}
}

func TestSignInPagesHaveSecurityHeadersAndNoEmailLeak(t *testing.T) {
	g := newRig(false)
	for name, w := range map[string]*httptest.ResponseRecorder{
		"form":  g.do("GET", "/dashboard/login"),
		"sent":  g.do("POST", "/dashboard/login", url.Values{"email": {"ana@acme.test"}}),
		"error": g.do("GET", "/dashboard/login/verify"),
	} {
		if !strings.HasPrefix(w.Header().Get("Content-Security-Policy"), "default-src 'none'") || w.Header().Get("X-Frame-Options") != "DENY" ||
			w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: security headers missing", name)
		}
		for _, want := range []string{`rel="icon"`, `href="/favicon.svg"`} {
			if !strings.Contains(w.Body.String(), want) {
				t.Errorf("%s lacks %s", name, want)
			}
		}
	}
	if strings.Contains(g.do("POST", "/dashboard/login", url.Values{"email": {"ana@acme.test"}}).Body.String(), "ana@acme.test") {
		t.Error("the sent page repeats the email address")
	}
}

func TestMailIsSentFromTheDashboardTemplate(t *testing.T) {
	g := newRig(false)
	g.do("POST", "/dashboard/login", url.Values{"email": {"ana@acme.test"}})
	m := g.box.sent[0]
	want := mail.DashboardLoginMessage("ana@acme.test", strings.TrimSpace(strings.Split(strings.Split(m.Text, "\n\n")[1], "\n")[0]), "https://foyr.test")
	if m.Subject != want.Subject || m.HTML != want.HTML {
		t.Error("the email is not the dashboard sign-in message")
	}
}
