package dashboard

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// asPerson serves the dashboard to p, with the same fake data as the other tests.
func asPerson(p Person) (http.Handler, *mem, *outbox) {
	m, box := newMem(), &outbox{}
	mux := http.NewServeMux()
	(&Handler{Data: fake{m: m}, Who: func(*http.Request) (Person, bool) { return p, true }, SignIn: "/founder", AppsDomain: "apps.test",
		AppsScheme: "https", Mail: box, Now: func() time.Time { return now }}).Register(mux)
	return mux, m, box
}

// ana owns acme (installation 1) and knows nothing of other-co (2).
var ana = Person{UserID: 7, Email: "ana@acme.test", Roles: map[int64]string{1: "owner"}}

func TestCustomerSeesOnlyTheirAccounts(t *testing.T) {
	h, _, _ := asPerson(ana)
	w := get(h, "/dashboard")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard/acme" {
		t.Fatalf("start: %d %q", w.Code, w.Header().Get("Location"))
	}
	body := get(h, "/dashboard/acme").Body.String()
	if !strings.Contains(body, "https://crm.apps.test") || strings.Contains(body, "other-co") || strings.Contains(body, "secret") {
		t.Fatalf("account page shows another account\n%s", body)
	}
	for _, tab := range Tabs {
		p := "/dashboard/acme/crm"
		if tab.Path != "" {
			p += "/" + tab.Path
		}
		if b := get(h, p).Body.String(); strings.Contains(b, "other-co") {
			t.Errorf("%s lists another account", p)
		}
	}
	// Someone linked only to other-co is sent there, not to acme.
	other, _, _ := asPerson(Person{UserID: 8, Email: "bob@other.test", Roles: map[int64]string{2: "owner"}})
	if w := get(other, "/dashboard"); w.Header().Get("Location") != "/dashboard/other-co" {
		t.Errorf("start for other-co: %q", w.Header().Get("Location"))
	}
}

func TestCustomerCannotReachAnotherAccount(t *testing.T) {
	h, _, _ := asPerson(ana)
	for _, p := range []string{
		"/dashboard/other-co", "/dashboard/other-co/secret", "/dashboard/other-co/secret/access", "/dashboard/other-co/secret/settings",
		"/dashboard/other-co/secret/deployments", "/dashboard/other-co/secret/deployments/201",
		"/dashboard/acme/secret", "/dashboard/acme/crm/deployments/201",
	} {
		w := get(h, p)
		if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "Other account's commit") {
			t.Errorf("GET %s: %d, want 404", p, w.Code)
		}
	}
}

func TestCustomerPostsStayInsideTheirAccounts(t *testing.T) {
	for _, path := range []string{
		"/dashboard/other-co/secret/access/invites", "/dashboard/other-co/secret/access/mode",
		"/dashboard/other-co/secret/access/invites/2/revoke", "/dashboard/other-co/secret/access/members/8/remove",
		"/dashboard/acme/crm/access/invites/2/revoke", "/dashboard/acme/crm/access/members/8/remove",
	} {
		h, m, box := asPerson(ana)
		w := post(h, path, url.Values{"email": {"a@b.test"}, "role": {"member"}, "mode": {"public"}}, same)
		if w.Code != http.StatusNotFound {
			t.Errorf("POST %s: %d, want 404", path, w.Code)
		}
		if len(m.invites[20]) != 1 || len(m.members[20]) != 1 || len(m.mode) != 0 || len(box.sent) != 0 || len(m.actors) != 0 {
			t.Errorf("POST %s changed something", path)
		}
	}
}

// A role in one account gives nothing in another.
func TestRolesAreCheckedPerAccount(t *testing.T) {
	mixed := Person{UserID: 7, Email: "ana@acme.test", Roles: map[int64]string{1: "owner", 2: "member"}}
	for _, c := range []struct {
		name   string
		person Person
		path   string
		code   int
	}{
		{"owner changes own account", mixed, "/dashboard/acme/crm/access/mode", http.StatusSeeOther},
		{"member cannot change", mixed, "/dashboard/other-co/secret/access/mode", http.StatusForbidden},
		{"member cannot invite", mixed, "/dashboard/other-co/secret/access/invites", http.StatusForbidden},
		{"member cannot revoke", mixed, "/dashboard/other-co/secret/access/invites/2/revoke", http.StatusForbidden},
		{"member cannot remove", mixed, "/dashboard/other-co/secret/access/members/8/remove", http.StatusForbidden},
		{"admin may change", Person{UserID: 9, Email: "adm@acme.test", Roles: map[int64]string{1: "admin"}}, "/dashboard/acme/crm/access/mode", http.StatusSeeOther},
		{"unknown role may not", Person{UserID: 9, Email: "x@acme.test", Roles: map[int64]string{1: "viewer"}}, "/dashboard/acme/crm/access/mode", http.StatusForbidden},
	} {
		h, m, _ := asPerson(c.person)
		w := post(h, c.path, url.Values{"email": {"a@b.test"}, "role": {"member"}, "mode": {"public"}}, same)
		if w.Code != c.code {
			t.Errorf("%s: %d, want %d", c.name, w.Code, c.code)
		}
		if c.code == http.StatusForbidden && (len(m.actors) != 0 || len(m.mode) != 0 || len(m.invites[20]) != 1 || len(m.members[20]) != 1) {
			t.Errorf("%s: a refused change went through", c.name)
		}
	}
	// A read-only member still sees the page.
	h, _, _ := asPerson(mixed)
	if w := get(h, "/dashboard/other-co/secret/access"); w.Code != 200 {
		t.Errorf("member reading access: %d", w.Code)
	}
}

func TestAuditNamesThePerson(t *testing.T) {
	h, m, _ := asPerson(ana)
	post(h, "/dashboard/acme/crm/access/mode", url.Values{"mode": {"public"}}, same)
	if len(m.actors) != 1 || m.actors[0] != (store.Actor{Type: "foyr_user", ID: "7", Login: "ana@acme.test"}) {
		t.Fatalf("actors = %+v", m.actors)
	}
	hf, mf, _ := asPerson(Person{Founder: true})
	post(hf, "/dashboard/acme/crm/access/mode", url.Values{"mode": {"public"}}, same)
	if len(mf.actors) != 1 || mf.actors[0] != (store.Actor{Type: "system", Login: "founder"}) {
		t.Fatalf("founder actors = %+v", mf.actors)
	}
}

func TestFounderBannerIsForTheFounderOnly(t *testing.T) {
	fh, _, _ := asPerson(Person{Founder: true})
	if !strings.Contains(get(fh, "/dashboard/acme").Body.String(), "as the founder") {
		t.Error("the founder lost the banner")
	}
	for _, p := range []string{"/dashboard/acme", "/dashboard/acme/crm", "/dashboard/nope"} {
		ch, _, _ := asPerson(ana)
		if b := get(ch, p).Body.String(); strings.Contains(b, "as the founder") || strings.Contains(b, `href="/founder"`) {
			t.Errorf("%s shows the founder banner to a customer", p)
		}
	}
}

func TestFounderStillSeesEverything(t *testing.T) {
	h, _, _ := asPerson(Person{Founder: true})
	for _, p := range []string{"/dashboard/acme", "/dashboard/other-co", "/dashboard/other-co/secret/access"} {
		if w := get(h, p); w.Code != 200 {
			t.Errorf("founder GET %s: %d", p, w.Code)
		}
	}
}

func TestPersonFromUser(t *testing.T) {
	p := PersonFromUser(store.DashboardUser{ID: 7, Email: "ana@acme.test", Accounts: []store.AccountAccess{
		{Installation: store.Installation{ID: 1}, Role: "owner"}, {Installation: store.Installation{ID: 2}, Role: "member"}}})
	if p.Founder || p.UserID != 7 || p.Email != "ana@acme.test" || p.Roles[1] != "owner" || p.Roles[2] != "member" || len(p.Roles) != 2 {
		t.Fatalf("person = %+v", p)
	}
}

func TestSignedOutStillGoesToSignIn(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{Data: fake{m: newMem()}, Who: func(*http.Request) (Person, bool) { return ana, false }, SignIn: "/signin", AppsDomain: "apps.test"}).Register(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/dashboard/acme", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/signin" {
		t.Fatalf("%d %q", w.Code, w.Header().Get("Location"))
	}
}
