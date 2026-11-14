package dashboard

import (
	"context"
	"errors"
	"html"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// ssoApps are the apps the fake knows: crm belongs to account 1, frozen to a suspended account.
var ssoApps = map[string]struct {
	inst      int64
	suspended bool
}{"crm": {1, false}, "frozen": {1, true}}

// IssueSSOCode lets an owner or admin of the app's live account in, as the store does.
func (a *fakeAuth) IssueSSOCode(_ context.Context, userID int64, slug string, _ time.Time) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ssoCalls++
	if slug == "boom" {
		return "", errors.New("db down")
	}
	app, ok := ssoApps[slug]
	if !ok || app.suspended {
		return "", store.ErrNoSSO
	}
	for _, u := range a.sessions {
		if u.ID != userID {
			continue
		}
		for _, acc := range u.Accounts {
			if acc.Installation.ID == app.inst && (acc.Role == "owner" || acc.Role == "admin") {
				return "sso-code", nil
			}
		}
	}
	return "", store.ErrNoSSO
}

const emailFormURL = "https://auth.foyr.test/login?app=crm&return=%2Freports&sso=0"

func ssoGet(g *loginRig, slug, ret string, parts ...any) (code int, location, body string) {
	w := g.do("GET", "/dashboard/sso?app="+slug+"&return="+ret, parts...)
	return w.Code, w.Header().Get("Location"), w.Body.String()
}

func TestSSOIssuesCode(t *testing.T) {
	g := newRig(false)
	code, loc, _ := ssoGet(g, "crm", "/reports", cookie(CookieName, "ana-cookie"))
	if code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", code)
	}
	if want := "https://crm.apps.test/__foyr/callback?code=sso-code&return=%2Freports"; loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

func TestSSOSignedOutGoesToTheEmailForm(t *testing.T) {
	for _, c := range []struct {
		name    string
		founder bool
		parts   []any
	}{
		{"no cookie", false, nil},
		{"unknown cookie", false, []any{cookie(CookieName, "garbage")}},
		{"founder cookie only", true, []any{cookie("founder", "yes")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := newRig(c.founder)
			code, loc, _ := ssoGet(g, "crm", "/reports", c.parts...)
			if code != http.StatusSeeOther || loc != emailFormURL {
				t.Errorf("got %d %q, want 303 %q", code, loc, emailFormURL)
			}
			if g.auth.ssoCalls != 0 {
				t.Errorf("IssueSSOCode called %d times, want 0", g.auth.ssoCalls)
			}
		})
	}
}

var hrefRE = regexp.MustCompile(`href="([^"]*sso=0[^"]*)"`)

func TestSSODeniedShowsOnePageForEveryReason(t *testing.T) {
	g := newRig(false)
	var bodies []string
	for _, slug := range []string{"crm", "nope", "frozen"} {
		code, _, body := ssoGet(g, slug, "/reports", cookie(CookieName, "mia-cookie")) // mia is a read-only member
		if code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403", slug, code)
		}
		if !strings.Contains(body, "mia@acme.test") || !strings.Contains(body, "owners and admins") {
			t.Errorf("%s: page does not say who is signed in and who can open it:\n%s", slug, body)
		}
		m := hrefRE.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("%s: no link to the email form in\n%s", slug, body)
		}
		if got, want := html.UnescapeString(m[1]), "https://auth.foyr.test/login?app="+slug+"&return=%2Freports&sso=0"; got != want {
			t.Errorf("%s: email form link = %q, want %q", slug, got, want)
		}
		bodies = append(bodies, strings.ReplaceAll(body, slug, "SLUG"))
	}
	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Errorf("denied pages differ by reason:\n%s\n---\n%s", bodies[0], bodies[i])
		}
	}
	// An owner of another account is refused the same way.
	if code, _, _ := ssoGet(g, "crm", "/reports", cookie(CookieName, "zed-cookie")); code != http.StatusForbidden {
		t.Errorf("owner of another account: status = %d, want 403", code)
	}
}

func TestSSOHostileInput(t *testing.T) {
	g := newRig(false)
	for _, slug := range []string{"evil.com/x", "A..B", "Crm", ""} {
		if code, loc, _ := ssoGet(g, slug, "/reports", cookie(CookieName, "ana-cookie")); code != http.StatusNotFound || loc != "" {
			t.Errorf("app=%q: %d %q, want 404 and no redirect", slug, code, loc)
		}
	}
	_, loc, _ := ssoGet(g, "crm", "//evil.test", cookie(CookieName, "ana-cookie"))
	if want := "https://crm.apps.test/__foyr/callback?code=sso-code&return=%2F"; loc != want {
		t.Errorf("return=//evil.test: Location = %q, want %q", loc, want)
	}
}

func TestSSOOnlyOnTheDashboardHostAndErrorsAreNotRedirects(t *testing.T) {
	g := newRig(false)
	if code, loc, _ := ssoGet(g, "crm", "/reports", cookie(CookieName, "ana-cookie"), host("api.foyr.test")); code != http.StatusNotFound || loc != "" {
		t.Errorf("other host: %d %q, want 404", code, loc)
	}
	if code, loc, _ := ssoGet(g, "boom", "/reports", cookie(CookieName, "ana-cookie")); code != http.StatusInternalServerError || loc != "" {
		t.Errorf("store error: %d %q, want 500 and no Location", code, loc)
	}
}
