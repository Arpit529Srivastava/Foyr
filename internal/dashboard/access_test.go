package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/mail"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// mem is the in-memory members, invites and modes behind the fake store.
type mem struct {
	members map[int64][]store.Member
	invites map[int64][]store.Invite
	mode    map[int64]string
	actors  []store.Actor
	nextID  int64
}

func newMem() *mem {
	return &mem{
		members: map[int64][]store.Member{10: {{UserID: 7, Email: "ana@acme.test", Role: "owner", CreatedAt: now.Add(-48 * time.Hour)}}, 20: {{UserID: 8, Email: "spy@other.test", Role: "member"}}},
		invites: map[int64][]store.Invite{10: {{ID: 1, AppID: 10, Email: "jon@client.test", Role: "viewer", CreatedAt: now.Add(-time.Hour)}}, 20: {{ID: 2, AppID: 20, Email: "x@other.test", Role: "member"}}},
		mode:    map[int64]string{},
		nextID:  100,
	}
}

func (f fake) ListMembers(_ context.Context, appID int64) ([]store.Member, error) {
	return f.m.members[appID], nil
}

func (f fake) ListInvites(_ context.Context, appID int64) ([]store.Invite, error) {
	return f.m.invites[appID], nil
}

func (f fake) CreateInvite(_ context.Context, appID int64, email, role string, by store.Actor) (store.Invite, error) {
	f.m.actors = append(f.m.actors, by)
	switch {
	case !strings.Contains(email, "@"):
		return store.Invite{}, store.ErrBadEmail
	case role != "owner" && role != "admin" && role != "member" && role != "viewer":
		return store.Invite{}, store.ErrBadRole
	}
	for _, m := range f.m.members[appID] {
		if m.Email == email {
			return store.Invite{}, store.ErrAlreadyMember
		}
	}
	f.m.nextID++
	inv := store.Invite{ID: f.m.nextID, AppID: appID, Email: email, Role: role, CreatedAt: now}
	f.m.invites[appID] = append(f.m.invites[appID], inv)
	return inv, nil
}

func (f fake) RevokeInvite(_ context.Context, id int64, by store.Actor) error {
	for app, list := range f.m.invites {
		for i, inv := range list {
			if inv.ID == id {
				f.m.actors = append(f.m.actors, by)
				f.m.invites[app] = append(list[:i:i], list[i+1:]...)
				return nil
			}
		}
	}
	return store.ErrNotFound
}

func (f fake) RemoveMember(_ context.Context, appID, userID int64, by store.Actor) error {
	list := f.m.members[appID]
	for i, m := range list {
		if m.UserID == userID {
			f.m.actors = append(f.m.actors, by)
			f.m.members[appID] = append(list[:i:i], list[i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}

func (f fake) SetAccessMode(_ context.Context, appID int64, mode string, by store.Actor) error {
	f.m.actors = append(f.m.actors, by)
	if mode != "private" && mode != "public" {
		return store.ErrBadMode
	}
	f.m.mode[appID] = mode
	return nil
}

type outbox struct{ sent []mail.Message }

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.sent = append(o.sent, m)
	return nil
}

func accessHandler(allowed bool) (http.Handler, *mem, *outbox) {
	m, box := newMem(), &outbox{}
	mux := http.NewServeMux()
	(&Handler{Data: fake{m: m}, Who: func(*http.Request) (Person, bool) { return Person{Founder: true}, allowed }, SignIn: "/founder", AppsDomain: "apps.test",
		AppsScheme: "https", Mail: box, Now: func() time.Time { return now }}).Register(mux)
	return mux, m, box
}

func post(h http.Handler, path string, form url.Values, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Host = "foyr.test"
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const same = "https://foyr.test"
const accessPath = "/dashboard/acme/crm/access"

func TestAccessTabShowsRealPeople(t *testing.T) {
	h, _, _ := accessHandler(true)
	w := get(h, accessPath)
	body := w.Body.String()
	if w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	for _, want := range []string{"ana@acme.test", "jon@client.test", "owner", "viewer"} {
		if !strings.Contains(body, want) {
			t.Errorf("access tab is missing %q", want)
		}
	}
	// Neither the old sample people nor another app's people.
	for _, bad := range []string{"ravi@acme.com", "meera@acme.com", "Preview.", "spy@other.test", "x@other.test"} {
		if strings.Contains(body, bad) {
			t.Errorf("access tab shows %q", bad)
		}
	}
}

func TestInviteSendsAnEmailAndShowsUp(t *testing.T) {
	h, m, box := accessHandler(true)
	w := post(h, accessPath+"/invites", url.Values{"email": {"new@acme.test"}, "role": {"member"}}, same)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != accessPath {
		t.Fatalf("%d %q", w.Code, w.Header().Get("Location"))
	}
	if len(box.sent) != 1 || box.sent[0].To != "new@acme.test" || !strings.Contains(box.sent[0].Text, "crm.apps.test") {
		t.Fatalf("email: %+v", box.sent)
	}
	if got := m.invites[10]; len(got) != 2 || got[1].Email != "new@acme.test" {
		t.Fatalf("invites: %+v", got)
	}
	if m.actors[0].Login != "founder" {
		t.Errorf("actor: %+v", m.actors[0])
	}
	if !strings.Contains(get(h, accessPath).Body.String(), "new@acme.test") {
		t.Error("new invite is not listed")
	}
}

func TestInviteProblemsAreExplained(t *testing.T) {
	for _, c := range []struct {
		name  string
		form  url.Values
		code  int
		reply string
	}{
		{"bad email", url.Values{"email": {"nope"}, "role": {"member"}}, 400, "not a valid email"},
		{"bad role", url.Values{"email": {"a@b.test"}, "role": {"god"}}, 400, "role must be"},
		{"already a member", url.Values{"email": {"ana@acme.test"}, "role": {"member"}}, 409, "already a member"},
	} {
		h, _, box := accessHandler(true)
		w := post(h, accessPath+"/invites", c.form, same)
		if w.Code != c.code || !strings.Contains(strings.ToLower(w.Body.String()), c.reply) {
			t.Errorf("%s: %d\n%s", c.name, w.Code, w.Body.String())
		}
		if len(box.sent) != 0 {
			t.Errorf("%s: an email went out", c.name)
		}
	}
}

func TestRevokeRemoveAndMode(t *testing.T) {
	h, m, _ := accessHandler(true)
	if w := post(h, accessPath+"/invites/1/revoke", nil, same); w.Code != http.StatusSeeOther {
		t.Fatalf("revoke: %d", w.Code)
	}
	if len(m.invites[10]) != 0 {
		t.Error("invite is still open")
	}
	if w := post(h, accessPath+"/members/7/remove", nil, same); w.Code != http.StatusSeeOther {
		t.Fatalf("remove: %d", w.Code)
	}
	if len(m.members[10]) != 0 {
		t.Error("member is still there")
	}
	if w := post(h, accessPath+"/mode", url.Values{"mode": {"public"}}, same); w.Code != http.StatusSeeOther || m.mode[10] != "public" {
		t.Fatalf("mode: %d %q", w.Code, m.mode[10])
	}
	if w := post(h, accessPath+"/mode", url.Values{"mode": {"open"}}, same); w.Code != http.StatusBadRequest {
		t.Fatalf("bad mode: %d", w.Code)
	}
}

// Another account's or another app's ids never work through this app's URL.
func TestAccessChangesStayInsideTheApp(t *testing.T) {
	for _, c := range []struct{ path string }{
		{"/dashboard/other-co/crm/access/invites"},          // crm is not other-co's
		{"/dashboard/acme/secret/access/invites"},           // secret is not acme's
		{accessPath + "/invites/2/revoke"},                  // invite 2 belongs to another app
		{accessPath + "/members/8/remove"},                  // user 8 belongs to another app
		{"/dashboard/acme/secret/access/mode"},              // another account's app
		{"/dashboard/other-co/crm/access/invites/1/revoke"}, // right invite, wrong account
		{"/dashboard/acme/crm/access/invites/x/revoke"},     // not a number
		{"/dashboard/acme/crm/access/members/-1/remove"},    // not a valid id
	} {
		h, m, box := accessHandler(true)
		before := len(m.invites[10]) + len(m.invites[20]) + len(m.members[10]) + len(m.members[20])
		w := post(h, c.path, url.Values{"email": {"a@b.test"}, "role": {"member"}, "mode": {"public"}}, same)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", c.path, w.Code)
		}
		after := len(m.invites[10]) + len(m.invites[20]) + len(m.members[10]) + len(m.members[20])
		if before != after || len(m.mode) != 0 || len(box.sent) != 0 {
			t.Errorf("%s changed something", c.path)
		}
	}
}

func TestAccessPostsNeedSignInAndSameOrigin(t *testing.T) {
	out, m, _ := accessHandler(false)
	if w := post(out, accessPath+"/invites/1/revoke", nil, same); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/founder" {
		t.Errorf("signed out: %d %q", w.Code, w.Header().Get("Location"))
	}
	in, m2, box := accessHandler(true)
	for _, origin := range []string{"", "null", "https://evil.test"} {
		if w := post(in, accessPath+"/invites", url.Values{"email": {"a@b.test"}, "role": {"member"}}, origin); w.Code != http.StatusForbidden {
			t.Errorf("origin %q: %d, want 403", origin, w.Code)
		}
	}
	if len(m.invites[10]) != 1 || len(m2.invites[10]) != 1 || len(box.sent) != 0 {
		t.Error("a refused post changed something")
	}
}

func TestInviteIsSavedEvenIfTheEmailFails(t *testing.T) {
	_, m, _ := accessHandler(true)
	mux := http.NewServeMux()
	(&Handler{Data: fake{m: m}, Who: func(*http.Request) (Person, bool) { return Person{Founder: true}, true }, SignIn: "/founder", AppsDomain: "apps.test",
		Mail: failing{}, Now: func() time.Time { return now }}).Register(mux)
	w := post(mux, accessPath+"/invites", url.Values{"email": {"new@acme.test"}, "role": {"member"}}, same)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "could not be sent") {
		t.Fatalf("%d\n%s", w.Code, w.Body.String())
	}
	if len(m.invites[10]) != 2 {
		t.Error("the invite was not kept")
	}
}

type failing struct{}

func (failing) Send(context.Context, mail.Message) error { return http.ErrAbortHandler }

func TestAccessTabIsNoLongerAPreview(t *testing.T) {
	for _, tab := range Tabs {
		if tab.Path == "access" && tab.Preview {
			t.Error("the Access tab shows real data now")
		}
	}
}

// On a phone a wide table would push Remove and Revoke off-screen, so the table is marked to stack.
func TestPeopleTableStacksOnPhones(t *testing.T) {
	if body := get(handler(true), "/dashboard/acme/crm/access").Body.String(); !strings.Contains(body, `<table class="rows">`) {
		t.Fatal(`the people table needs class="rows"`)
	}
}
