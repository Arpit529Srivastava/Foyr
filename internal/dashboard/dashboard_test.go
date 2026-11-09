package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

var now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

type fake struct{ m *mem }

func at(min int) *time.Time { t := now.Add(time.Duration(min) * time.Minute); return &t }

var (
	accounts = []store.Installation{{ID: 1, Login: "acme"}, {ID: 2, Login: "other-co"}}
	apps     = []store.App{
		{ID: 10, InstallationID: 1, FullName: "acme/crm", DefaultBranch: "main", Slug: "crm", LiveDeployment: 101},
		{ID: 11, InstallationID: 1, FullName: "acme/api", DefaultBranch: "main", Slug: "api"},
		{ID: 20, InstallationID: 2, FullName: "other-co/secret", DefaultBranch: "main", Slug: "secret"},
	}
	deps = []store.Deployment{
		{ID: 102, AppID: 10, InstallationID: 1, CommitSHA: strings.Repeat("b", 40), Branch: "main", CommitMessage: "Break the health check",
			PusherLogin: "ana", Status: store.StatusFailed, FailureStep: "Health check", FailureReason: "no 2xx from / within 6 minutes", CreatedAt: now.Add(-10 * time.Minute)},
		{ID: 101, AppID: 10, InstallationID: 1, CommitSHA: strings.Repeat("a", 40), Branch: "main", CommitMessage: "Add reports\n\nlong body",
			PusherLogin: "ana", Status: store.StatusLive, CreatedAt: now.Add(-time.Hour), LiveAt: at(-58)},
		{ID: 201, AppID: 20, InstallationID: 2, CommitSHA: strings.Repeat("c", 40), Branch: "main", CommitMessage: "Other account's commit",
			Status: store.StatusLive, CreatedAt: now.Add(-time.Hour)},
	}
)

func (fake) ListInstallations(context.Context) ([]store.Installation, error) { return accounts, nil }
func (fake) ListApps(context.Context) ([]store.App, error)                   { return apps, nil }
func (fake) ListDeployments(_ context.Context, appID, _ int64, limit int) ([]store.Deployment, error) {
	var out []store.Deployment
	for _, d := range deps {
		if d.AppID == appID && len(out) < limit {
			out = append(out, d)
		}
	}
	return out, nil
}
func (fake) GetDeployment(_ context.Context, id int64) (store.Deployment, error) {
	for _, d := range deps {
		if d.ID == id {
			return d, nil
		}
	}
	return store.Deployment{}, store.ErrNotFound
}
func (fake) Steps(context.Context, int64) ([]store.Step, error) {
	return []store.Step{{Name: "Clone", Status: store.StepSucceeded, StartedAt: at(-60), FinishedAt: at(-59)}}, nil
}
func (fake) Timeline(context.Context, int64) ([]store.DeploymentEvent, error) { return nil, nil }

func handler(allowed bool) http.Handler {
	mux := http.NewServeMux()
	(&Handler{Data: fake{m: newMem()}, Who: func(*http.Request) (Person, bool) { return Person{Founder: true}, allowed }, SignIn: "/founder",
		AppsDomain: "apps.test", Now: func() time.Time { return now }}).Register(mux)
	return mux
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

func TestSignedOutGoesToSignIn(t *testing.T) {
	for _, p := range []string{"/dashboard", "/dashboard/acme", "/dashboard/acme/crm", "/dashboard/acme/crm/deployments/101"} {
		if w := get(handler(false), p); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/founder" {
			t.Errorf("%s: %d %q", p, w.Code, w.Header().Get("Location"))
		}
	}
}

func TestDashboardOpensTheFirstAccount(t *testing.T) {
	if w := get(handler(true), "/dashboard"); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard/acme" {
		t.Fatalf("%d %q", w.Code, w.Header().Get("Location"))
	}
}

func TestAccountPage(t *testing.T) {
	w := get(handler(true), "/dashboard/ACME")
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "https://crm.apps.test") || !strings.Contains(body, ">api<") {
		t.Fatalf("%d\n%s", w.Code, body)
	}
	// The switcher lists every account, but another account's apps never show.
	if !strings.Contains(body, "other-co") || strings.Contains(body, "secret") {
		t.Fatal("account page leaks or misses accounts")
	}
}

func TestAppsAreScopedToTheirAccount(t *testing.T) {
	for _, p := range []string{
		"/dashboard/acme/secret", "/dashboard/other-co/crm", "/dashboard/nobody", "/dashboard/acme/crm/deployments/201",
		"/dashboard/acme/crm/deployments/x", "/dashboard/acme/crm/nope",
	} {
		if w := get(handler(true), p); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", p, w.Code)
		}
	}
}

func TestOverview(t *testing.T) {
	w := get(handler(true), "/dashboard/acme/crm")
	body := w.Body.String()
	for _, want := range []string{"https://crm.apps.test", "Add reports", "aaaaaaa", "ana", "2m", "Break the health check"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview is missing %q", want)
		}
	}
	if strings.Contains(body, "long body") {
		t.Error("only the first line of a commit message is shown")
	}
}

func TestDeploymentPageExplainsFailures(t *testing.T) {
	w := get(handler(true), "/dashboard/acme/crm/deployments/102")
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "no 2xx from / within 6 minutes") || !strings.Contains(body, "PORT") {
		t.Fatalf("%d\n%s", w.Code, body)
	}
}

func TestDeploymentsFilter(t *testing.T) {
	body := get(handler(true), "/dashboard/acme/crm/deployments?status=failed").Body.String()
	if !strings.Contains(body, "Break the health check") || strings.Contains(body, "Add reports") {
		t.Fatal("status filter did not filter")
	}
}

func TestEveryTabRendersSafely(t *testing.T) {
	for _, tab := range Tabs {
		p := "/dashboard/acme/crm"
		if tab.Path != "" {
			p += "/" + tab.Path
		}
		w := get(handler(true), p)
		if w.Code != 200 {
			t.Errorf("%s: %d", p, w.Code)
		}
		if tab.Preview && !strings.Contains(w.Body.String(), "Preview") {
			t.Errorf("%s: a tab without real data must say it's a preview", p)
		}
		if !strings.HasPrefix(w.Header().Get("Content-Security-Policy"), "default-src 'none'") || w.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: security headers missing", p)
		}
	}
}

func TestHint(t *testing.T) {
	for _, c := range []struct{ step, reason, want string }{
		{"Health check", "no 2xx", "PORT"},
		{"Build image", "exit 1", "Dockerfile"},
		{"Clone", "auth failed", "GitHub App"},
		{"", "", ""},
	} {
		if got := hint(c.step, c.reason); !strings.Contains(got, c.want) || (c.want == "" && got != "") {
			t.Errorf("hint(%q) = %q, want it to mention %q", c.step, got, c.want)
		}
	}
}

type noAccounts struct{ fake }

func (noAccounts) ListInstallations(context.Context) ([]store.Installation, error) { return nil, nil }

// Every page head names the icon, so tabs show the Foyr mark and not a grey globe.
func TestEveryPageLinksTheFavicon(t *testing.T) {
	empty := http.NewServeMux()
	(&Handler{Data: noAccounts{}, Who: func(*http.Request) (Person, bool) { return Person{Founder: true}, true }, SignIn: "/founder", AppsDomain: "apps.test",
		Now: func() time.Time { return now }}).Register(empty)
	for name, w := range map[string]*httptest.ResponseRecorder{
		"account page":   get(handler(true), "/dashboard/acme"),
		"app overview":   get(handler(true), "/dashboard/acme/crm"),
		"not found page": get(handler(true), "/dashboard/nope"),
		"no accounts":    get(empty, "/dashboard"),
	} {
		body := w.Body.String()
		for _, want := range []string{`rel="icon"`, `href="/favicon.svg"`, `href="/favicon.png"`, `rel="apple-touch-icon"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s (status %d) lacks %s", name, w.Code, want)
			}
		}
	}
}
