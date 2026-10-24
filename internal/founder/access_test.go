package founder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/mail"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// accessFake adds the members, invites and access mode calls to the page fake and records them.
type accessFake struct {
	fake
	mu        sync.Mutex
	mode      string
	members   []store.Member
	invites   []store.Invite
	calls     []string
	createErr error
}

func (a *accessFake) rec(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, s)
}

func (a *accessFake) GetApp(context.Context, int64) (store.App, error) {
	return store.App{ID: 1, Slug: "foyr-hello", FullName: "octo/foyr-hello", AccessMode: a.mode}, nil
}

func (a *accessFake) ListMembers(context.Context, int64) ([]store.Member, error) {
	return a.members, nil
}
func (a *accessFake) ListInvites(context.Context, int64) ([]store.Invite, error) {
	return a.invites, nil
}

func (a *accessFake) CreateInvite(_ context.Context, appID int64, email, role string, by store.Actor) (store.Invite, error) {
	a.rec("invite " + email + " " + role + " by " + by.Login)
	if a.createErr != nil {
		return store.Invite{}, a.createErr
	}
	return store.Invite{ID: 9, AppID: appID, Email: strings.ToLower(strings.TrimSpace(email)), Role: role}, nil
}

func (a *accessFake) RevokeInvite(_ context.Context, id int64, by store.Actor) error {
	a.rec("revoke " + itoa(id) + " by " + by.Login)
	return nil
}

func (a *accessFake) RemoveMember(_ context.Context, appID, userID int64, by store.Actor) error {
	a.rec("remove " + itoa(userID) + " by " + by.Login)
	return nil
}

func (a *accessFake) SetAccessMode(_ context.Context, _ int64, mode string, by store.Actor) error {
	a.rec("mode " + mode + " by " + by.Login)
	if mode != "private" && mode != "public" {
		return store.ErrBadMode
	}
	return nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

type outbox struct {
	mu   sync.Mutex
	sent []mail.Message
	err  error
}

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.err != nil {
		return o.err
	}
	o.sent = append(o.sent, m)
	return nil
}

func accessHandler(af *accessFake, ob *outbox) http.Handler {
	h := &Handler{Data: af, Mail: ob, Key: key, AppsDomain: "apps.example.test", Now: func() time.Time { return t0.Add(time.Hour) }, FailDelay: -1}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func postFrom(h http.Handler, path string, form url.Values, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAppPageShowsAccess(t *testing.T) {
	af := &accessFake{mode: "private",
		members: []store.Member{{UserID: 3, Email: "ana@example.com", Role: "admin"}},
		invites: []store.Invite{{ID: 9, Email: "bob@example.com", Role: "member"}}}
	h := accessHandler(af, &outbox{})

	if w := do(h, "GET", "/founder/apps/1", nil, nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/founder" {
		t.Fatalf("signed out: %d %q, want a redirect to sign in", w.Code, w.Header().Get("Location"))
	}
	w := do(h, "GET", "/founder/apps/1", nil, signIn(t, h))
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	for _, want := range []string{"foyr-hello", "private", "ana@example.com", "admin", "bob@example.com",
		`action="/founder/apps/1/invites"`, `action="/founder/apps/1/access"`, `action="/founder/apps/1/invites/9/revoke"`,
		`action="/founder/apps/1/members/3/remove"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if w := do(h, "GET", "/founder/apps/abc", nil, signIn(t, h)); w.Code != http.StatusNotFound {
		t.Errorf("bad id: %d, want 404", w.Code)
	}
}

func TestInvitePostNeedsSignIn(t *testing.T) {
	af, ob := &accessFake{mode: "private"}, &outbox{}
	h := accessHandler(af, ob)
	w := postFrom(h, "/founder/apps/1/invites", url.Values{"email": {"ana@example.com"}, "role": {"member"}}, "http://example.com", nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/founder" {
		t.Fatalf("signed out: %d %q, want a redirect to sign in", w.Code, w.Header().Get("Location"))
	}
	if len(af.calls) != 0 || len(ob.sent) != 0 {
		t.Errorf("a signed-out post did something: %v, %d emails", af.calls, len(ob.sent))
	}
}

func TestInviteCreatesAndEmails(t *testing.T) {
	af, ob := &accessFake{mode: "private"}, &outbox{}
	h := accessHandler(af, ob)
	c := signIn(t, h)
	w := do(h, "POST", "/founder/apps/1/invites", url.Values{"email": {" Ana@Example.com "}, "role": {"member"}}, c)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/founder/apps/1" {
		t.Fatalf("status %d %q", w.Code, w.Header().Get("Location"))
	}
	if len(af.calls) != 1 || af.calls[0] != "invite  Ana@Example.com  member by founder" {
		t.Fatalf("calls = %q", af.calls)
	}
	if len(ob.sent) != 1 {
		t.Fatalf("%d emails, want 1", len(ob.sent))
	}
	m := ob.sent[0]
	if m.To != "ana@example.com" || !strings.Contains(m.Subject, "foyr-hello") ||
		!strings.Contains(m.Text, "invited you to foyr-hello") || !strings.Contains(m.Text, "https://foyr-hello.apps.example.test/") {
		t.Errorf("email = %+v", m)
	}
}

func TestInviteEmailUsesTheFoyrDesign(t *testing.T) {
	af, ob := &accessFake{mode: "private"}, &outbox{}
	h := &Handler{Data: af, Mail: ob, Key: key, AppsDomain: "apps.example.test", SiteURL: "https://foyr.test", Now: func() time.Time { return t0.Add(time.Hour) }, FailDelay: -1}
	mux := http.NewServeMux()
	h.Register(mux)
	if w := do(mux, "POST", "/founder/apps/1/invites", url.Values{"email": {"ana@example.com"}, "role": {"owner"}}, signIn(t, mux)); w.Code != http.StatusSeeOther {
		t.Fatalf("status %d", w.Code)
	}
	m := ob.sent[0]
	for _, want := range []string{"https://foyr.test/email/banner.jpg", "You&#39;re invited to foyr-hello.", `href="https://foyr-hello.apps.example.test/"`, "owner"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
}

func TestInviteErrorsShowOnThePage(t *testing.T) {
	for _, c := range []struct {
		err    error
		status int
		text   string
	}{
		{store.ErrBadRole, http.StatusBadRequest, "Role must be"},
		{store.ErrBadEmail, http.StatusBadRequest, "Not a valid email"},
		{store.ErrAlreadyMember, http.StatusConflict, "already a member"},
	} {
		af, ob := &accessFake{mode: "private", createErr: c.err}, &outbox{}
		h := accessHandler(af, ob)
		w := do(h, "POST", "/founder/apps/1/invites", url.Values{"email": {"ana@example.com"}, "role": {"god"}}, signIn(t, h))
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.text) {
			t.Errorf("%v: %d, body has text %v", c.err, w.Code, strings.Contains(w.Body.String(), c.text))
		}
		if len(ob.sent) != 0 {
			t.Errorf("%v: an email was sent", c.err)
		}
	}
}

func TestInviteSavedButEmailFailed(t *testing.T) {
	af, ob := &accessFake{mode: "private"}, &outbox{err: context.DeadlineExceeded}
	h := accessHandler(af, ob)
	w := do(h, "POST", "/founder/apps/1/invites", url.Values{"email": {"ana@example.com"}, "role": {"member"}}, signIn(t, h))
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "saved") {
		t.Errorf("%d %s, want 502 saying the invite was saved but the email failed", w.Code, w.Body.String())
	}
}

func TestAccessChangesAreRecorded(t *testing.T) {
	af := &accessFake{mode: "private"}
	h := accessHandler(af, &outbox{})
	c := signIn(t, h)
	for _, p := range []struct {
		path string
		form url.Values
	}{
		{"/founder/apps/1/access", url.Values{"mode": {"public"}}},
		{"/founder/apps/1/invites/9/revoke", nil},
		{"/founder/apps/1/members/3/remove", nil},
	} {
		if w := do(h, "POST", p.path, p.form, c); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/founder/apps/1" {
			t.Errorf("%s: %d %q", p.path, w.Code, w.Header().Get("Location"))
		}
	}
	want := []string{"mode public by founder", "revoke 9 by founder", "remove 3 by founder"}
	if strings.Join(af.calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %q, want %q", af.calls, want)
	}
	if w := do(h, "POST", "/founder/apps/1/access", url.Values{"mode": {"secret"}}, c); w.Code != http.StatusBadRequest {
		t.Errorf("bad mode: %d, want 400", w.Code)
	}
}

func TestAccessPostsNeedSameOrigin(t *testing.T) {
	af, ob := &accessFake{mode: "private"}, &outbox{}
	h := accessHandler(af, ob)
	c := signIn(t, h)
	for _, p := range []string{"/founder/apps/1/invites", "/founder/apps/1/access", "/founder/apps/1/invites/9/revoke", "/founder/apps/1/members/3/remove"} {
		form := url.Values{"email": {"a@b.co"}, "role": {"member"}, "mode": {"public"}}
		for _, origin := range []string{"https://evil.com", ""} {
			if w := postFrom(h, p, form, origin, c); w.Code != http.StatusForbidden {
				t.Errorf("%s with Origin %q: %d, want 403", p, origin, w.Code)
			}
		}
	}
	if len(af.calls) != 0 || len(ob.sent) != 0 {
		t.Errorf("a cross-site post got through: %v", af.calls)
	}
}

func TestFounderPagesLinkTheFavicon(t *testing.T) {
	h := accessHandler(&accessFake{mode: "private"}, &outbox{})
	c := signIn(t, h)
	for name, w := range map[string]*httptest.ResponseRecorder{
		"sign-in page": do(h, "GET", "/founder", nil, nil),
		"overview":     do(h, "GET", "/founder", nil, c),
		"access page":  do(h, "GET", "/founder/apps/1", nil, c),
	} {
		body := w.Body.String()
		for _, want := range []string{`rel="icon"`, `href="/favicon.svg"`, `href="/favicon.png"`, `rel="apple-touch-icon"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %s", name, want)
			}
		}
	}
}
