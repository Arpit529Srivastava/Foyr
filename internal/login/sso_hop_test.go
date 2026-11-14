package login

import (
	"net/http"
	"testing"
)

const dashURL = "https://app.foyr.test"

// newSSORig is newRig with the dashboard host set, so the login page hops to dashboard SSO.
func newSSORig(t *testing.T, dash string) *rig {
	t.Helper()
	g := newRig(t)
	g.h.DashURL = dash
	return g
}

func TestLoginHopsToDashboardSSO(t *testing.T) {
	g := newSSORig(t, dashURL)
	w := g.get("/login?app=crm&return=/reports")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	if got, want := w.Header().Get("Location"), dashURL+"/dashboard/sso?app=crm&return=%2Freports"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestLoginHopKeepsReturnOnTheApp(t *testing.T) {
	g := newSSORig(t, dashURL)
	w := g.get("/login?app=crm&return=//evil.test")
	if got, want := w.Header().Get("Location"), dashURL+"/dashboard/sso?app=crm&return=%2F"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestLoginSkipsTheHop(t *testing.T) {
	for _, c := range []struct{ name, dash, target string }{
		{"sso=0", dashURL, "/login?app=crm&return=/reports&sso=0"},
		{"hostile slug", dashURL, "/login?app=evil.com/x"},
		{"uppercase slug", dashURL, "/login?app=A..B"},
		{"no app", dashURL, "/login"},
		{"dashboard off", "", "/login?app=crm"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := newSSORig(t, c.dash)
			w := g.get(c.target)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d (Location %q), want 200 with the form", w.Code, w.Header().Get("Location"))
			}
		})
	}
}

func TestLoginHopOnlyOnTheAuthHost(t *testing.T) {
	g := newSSORig(t, dashURL)
	w := g.do("GET", "/login?app=crm", "evil.test", "", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 on another host", w.Code)
	}
}
