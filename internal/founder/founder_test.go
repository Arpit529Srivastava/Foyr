package founder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

const key = "0123456789abcdef0123456789abcdef"

var t0 = time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)

type fake struct{ status string }

func (f fake) ListApps(context.Context) ([]store.App, error) {
	return []store.App{{ID: 1, FullName: "octo/foyr-hello", Slug: "foyr-hello", LiveDeployment: 5}}, nil
}

func (f fake) RecentDeployments(context.Context, int) ([]store.AppDeployment, error) {
	live := t0.Add(112 * time.Second)
	return []store.AppDeployment{{Deployment: store.Deployment{ID: 5, AppID: 1, Status: f.status, CommitSHA: "938473bf40b4",
		CommitMessage: "feat: <b>status</b> card\nbody", CreatedAt: t0, LiveAt: &live, PusherLogin: "octo"}, AppName: "octo/foyr-hello", Slug: "foyr-hello"}}, nil
}

func (f fake) ListDeployments(_ context.Context, appID, _ int64, _ int) ([]store.Deployment, error) {
	if appID != 1 {
		return nil, nil
	}
	return []store.Deployment{{ID: 5, AppID: 1, Status: f.status, CommitSHA: "938473bf40b4", CommitMessage: "only app one", CreatedAt: t0}}, nil
}

func (f fake) GetDeployment(_ context.Context, id int64) (store.Deployment, error) {
	if id != 5 {
		return store.Deployment{}, store.ErrNotFound
	}
	return store.Deployment{ID: 5, AppID: 1, Status: f.status, CommitSHA: "938473bf40b4", CommitMessage: "<script>x</script>",
		FailureStep: "Deploy", FailureReason: "health check failed", CreatedAt: t0}, nil
}

func (f fake) GetApp(context.Context, int64) (store.App, error) {
	return store.App{ID: 1, Slug: "foyr-hello"}, nil
}

func (f fake) Steps(context.Context, int64) ([]store.Step, error) {
	end := t0.Add(50 * time.Second)
	return []store.Step{{Name: "Build image", Status: store.StepSucceeded, StartedAt: &t0, FinishedAt: &end}}, nil
}

func (f fake) Timeline(context.Context, int64) ([]store.DeploymentEvent, error) {
	return []store.DeploymentEvent{{OccurredAt: t0, ToStatus: "queued", Message: "push received"}}, nil
}

func (f fake) ListAudit(context.Context, store.AuditFilter) ([]store.AuditEvent, error) {
	return []store.AuditEvent{{OccurredAt: t0, Action: "deployment.created", Actor: store.Actor{Type: "github_user", Login: "octo"}}}, nil
}

func newHandler(k, status string) http.Handler {
	h := &Handler{Data: fake{status: status}, Key: k, AppsDomain: "apps.example.test", Now: func() time.Time { return t0.Add(time.Hour) }}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func do(h http.Handler, method, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, path, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func signIn(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	w := do(h, "POST", "/founder/login", url.Values{"key": {key}}, nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/founder" {
		t.Fatalf("login = %d %q", w.Code, w.Header().Get("Location"))
	}
	cs := w.Result().Cookies()
	if len(cs) != 1 {
		t.Fatalf("cookies = %v", cs)
	}
	return cs[0]
}

func TestOffWithoutALongKey(t *testing.T) {
	for _, k := range []string{"", "short"} {
		h := newHandler(k, "live")
		if w := do(h, "GET", "/founder", nil, nil); w.Code != http.StatusNotFound {
			t.Errorf("key %q: GET = %d, want 404", k, w.Code)
		}
		if w := do(h, "POST", "/founder/login", url.Values{"key": {k}}, nil); w.Code != http.StatusNotFound {
			t.Errorf("key %q: login = %d, want 404", k, w.Code)
		}
	}
}

func TestSignedOutSeesOnlyTheForm(t *testing.T) {
	w := do(newHandler(key, "live"), "GET", "/founder", nil, nil)
	b := w.Body.String()
	if w.Code != 200 || !strings.Contains(b, `action="/founder/login"`) || strings.Contains(b, "foyr-hello") {
		t.Fatalf("signed out: %d %s", w.Code, b)
	}
}

func TestWrongKeyIsRefused(t *testing.T) {
	w := do(newHandler(key, "live"), "POST", "/founder/login", url.Values{"key": {key + "x"}}, nil)
	if w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 {
		t.Fatalf("wrong key: %d cookies=%v", w.Code, w.Result().Cookies())
	}
}

func TestCookieIsStrictAndNeverTheKey(t *testing.T) {
	c := signIn(t, newHandler(key, "live"))
	if c.Name != CookieName || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("cookie = %+v", c)
	}
	if c.Value == key || strings.Contains(c.Value, key) || c.MaxAge != int(SessionTTL.Seconds()) {
		t.Fatalf("cookie value or age wrong: %+v", c)
	}
}

func TestOldCookieStopsWorkingWithANewKey(t *testing.T) {
	c := signIn(t, newHandler(key, "live"))
	w := do(newHandler(strings.Repeat("z", 32), "live"), "GET", "/founder", nil, c)
	if strings.Contains(w.Body.String(), "foyr-hello") {
		t.Fatal("cookie from the old key still works")
	}
}

func TestHomeShowsAppsAndDeployments(t *testing.T) {
	h := newHandler(key, "live")
	w := do(h, "GET", "/founder", nil, signIn(t, h))
	b := w.Body.String()
	for _, want := range []string{`https://foyr-hello.apps.example.test`, `/founder/deployments/5`, "1m52s", "foyr-hello, 1h ago",
		"#5 feat: &lt;b&gt;status&lt;/b&gt; card", "Median push to live", "100%</b>reached live", "Build image", `class="on" aria-current="page"><svg`} {
		if !strings.Contains(b, want) {
			t.Errorf("home missing %q", want)
		}
	}
	if strings.Contains(b, "<b>status</b>") || strings.Contains(b, `http-equiv="refresh"`) {
		t.Error("home must escape commit messages and not refresh when nothing is running")
	}
}

func TestRefreshesWhileDeploying(t *testing.T) {
	h := &Handler{Data: fake{status: "deploying"}, Key: key, Now: func() time.Time { return t0.Add(2 * time.Minute) }}
	mux := http.NewServeMux()
	h.Register(mux)
	c := signIn(t, mux)
	if b := do(mux, "GET", "/founder", nil, c).Body.String(); !strings.Contains(b, `http-equiv="refresh"`) {
		t.Fatal("home should refresh while a deployment is running")
	}
	if b := do(mux, "GET", "/founder/deployments/5", nil, c).Body.String(); !strings.Contains(b, `http-equiv="refresh"`) {
		t.Fatal("deployment page should refresh while it is running")
	}
}

// Old deployments stuck in a running status (before deploys existed) must not refresh the page forever.
func TestNoRefreshForStaleDeployments(t *testing.T) {
	h := newHandler(key, "built")
	c := signIn(t, h)
	for _, path := range []string{"/founder", "/founder/deployments/5"} {
		if b := do(h, "GET", path, nil, c).Body.String(); strings.Contains(b, `http-equiv="refresh"`) {
			t.Errorf("%s refreshes for a deployment created an hour ago", path)
		}
	}
}

func TestDeploymentPage(t *testing.T) {
	h := newHandler(key, "failed")
	c := signIn(t, h)
	w := do(h, "GET", "/founder/deployments/5", nil, c)
	b := w.Body.String()
	for _, want := range []string{"Deployment #5", "Failed at Deploy", "health check failed", "Build image", "50s", "push received",
		"deployment.created", "&lt;script&gt;"} {
		if !strings.Contains(b, want) {
			t.Errorf("deployment page missing %q", want)
		}
	}
	if strings.Contains(b, "<script>") {
		t.Error("commit message must be escaped")
	}
	if w := do(h, "GET", "/founder/deployments/6", nil, c); w.Code != http.StatusNotFound {
		t.Errorf("missing deployment = %d", w.Code)
	}
	if w := do(h, "GET", "/founder/deployments/x", nil, c); w.Code != http.StatusNotFound {
		t.Errorf("bad id = %d", w.Code)
	}
}

func TestDeploymentNeedsSignIn(t *testing.T) {
	w := do(newHandler(key, "live"), "GET", "/founder/deployments/5", nil, &http.Cookie{Name: CookieName, Value: "forged"})
	if w.Code != http.StatusSeeOther || strings.Contains(w.Body.String(), "health check") {
		t.Fatalf("forged cookie: %d", w.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	w := do(newHandler(key, "live"), "GET", "/founder", nil, nil)
	for name, want := range map[string]string{
		"Content-Security-Policy": "default-src 'none'", "Cache-Control": "no-store", "Referrer-Policy": "no-referrer",
		"X-Robots-Tag": "noindex", "X-Content-Type-Options": "nosniff",
	} {
		if !strings.Contains(w.Header().Get(name), want) {
			t.Errorf("%s = %q, want %q", name, w.Header().Get(name), want)
		}
	}
}

func TestLogoutClearsTheCookie(t *testing.T) {
	w := do(newHandler(key, "live"), "POST", "/founder/logout", nil, nil)
	cs := w.Result().Cookies()
	if w.Code != http.StatusSeeOther || len(cs) != 1 || cs[0].MaxAge >= 0 {
		t.Fatalf("logout: %d %v", w.Code, cs)
	}
}

func TestFmtDur(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "-", 850 * time.Millisecond: "850ms", 50 * time.Second: "50s",
		112 * time.Second: "1m52s", 120 * time.Second: "2m", 3*time.Hour + 4*time.Minute: "3h4m0s"} {
		if got := fmtDur(d); got != want {
			t.Errorf("fmtDur(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestSidebarOnEveryPage(t *testing.T) {
	h := newHandler(key, "live")
	c := signIn(t, h)
	for _, path := range []string{"/founder", "/founder/deployments", "/founder/activity", "/founder/deployments/5"} {
		b := do(h, "GET", path, nil, c).Body.String()
		for _, want := range []string{`<aside`, `href="/founder/deployments?app=1"`, `href="/founder/activity"`, "apps.example.test"} {
			if !strings.Contains(b, want) {
				t.Errorf("%s: sidebar missing %q", path, want)
			}
		}
	}
	if b := do(h, "GET", "/founder", nil, nil).Body.String(); strings.Contains(b, "<aside") {
		t.Error("the sign-in page must not show the sidebar")
	}
}

func TestDeploymentsPage(t *testing.T) {
	h := newHandler(key, "failed")
	c := signIn(t, h)
	all := do(h, "GET", "/founder/deployments", nil, c)
	if all.Code != 200 || !strings.Contains(all.Body.String(), "feat: &lt;b&gt;status&lt;/b&gt; card") {
		t.Fatalf("all deployments: %d", all.Code)
	}
	one := do(h, "GET", "/founder/deployments?app=1", nil, c).Body.String()
	if !strings.Contains(one, "only app one") || strings.Contains(one, "status&lt;/b&gt; card") {
		t.Error("?app=1 must list only that app's deployments")
	}
	if w := do(h, "GET", "/founder/deployments?app=x", nil, c); w.Code != http.StatusNotFound {
		t.Errorf("bad app filter = %d", w.Code)
	}
	if w := do(h, "GET", "/founder/deployments", nil, nil); w.Code != http.StatusSeeOther {
		t.Errorf("signed out = %d", w.Code)
	}
}

func TestActivityPage(t *testing.T) {
	h := newHandler(key, "live")
	b := do(h, "GET", "/founder/activity", nil, signIn(t, h)).Body.String()
	if !strings.Contains(b, "deployment.created") || !strings.Contains(b, "octo") {
		t.Fatal("activity page should list audit events")
	}
	if w := do(h, "GET", "/founder/activity", nil, nil); w.Code != http.StatusSeeOther {
		t.Errorf("signed out = %d", w.Code)
	}
}
