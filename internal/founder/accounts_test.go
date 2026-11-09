package founder

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

func (fake) ListInstallations(context.Context) ([]store.Installation, error) {
	return []store.Installation{{ID: 1, Login: "acme"}, {ID: 2, Login: "other-co"}}, nil
}

func (fake) ListAccountMembers(_ context.Context, inst int64) ([]store.AccountMember, error) {
	if inst == 1 {
		return []store.AccountMember{{UserID: 7, Email: "ana@acme.test", Role: "owner", CreatedAt: t0}}, nil
	}
	return nil, nil
}

func (fake) LinkAccountMember(context.Context, int64, string, string, store.Actor) (store.AccountMember, error) {
	return store.AccountMember{}, nil
}
func (fake) UnlinkAccountMember(context.Context, int64, int64, store.Actor) error { return nil }

// linkRec records what the page asked the store to do.
type linkRec struct {
	fake
	calls []string
	by    []store.Actor
	err   error
}

func (l *linkRec) LinkAccountMember(_ context.Context, inst int64, email, role string, by store.Actor) (store.AccountMember, error) {
	l.calls, l.by = append(l.calls, "link "+itoa(inst)+" "+email+" "+role), append(l.by, by)
	return store.AccountMember{}, l.err
}

func (l *linkRec) UnlinkAccountMember(_ context.Context, inst, uid int64, by store.Actor) error {
	l.calls, l.by = append(l.calls, "unlink "+itoa(inst)+" "+itoa(uid)), append(l.by, by)
	return l.err
}

func accountsHandler(d Data) http.Handler {
	h := &Handler{Data: d, Key: key, AppsDomain: "apps.example.test", Now: func() time.Time { return t0.Add(time.Hour) }, FailDelay: -1}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func TestAccountsPageListsPeopleAndForms(t *testing.T) {
	h := accountsHandler(fake{status: "live"})
	if w := do(h, "GET", "/founder/accounts", nil, nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/founder" {
		t.Fatalf("signed out: %d %q", w.Code, w.Header().Get("Location"))
	}
	w := do(h, "GET", "/founder/accounts", nil, signIn(t, h))
	b := w.Body.String()
	if w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	for _, want := range []string{"acme", "other-co", "ana@acme.test", "owner",
		`action="/founder/accounts/1/people"`, `action="/founder/accounts/2/people"`, `action="/founder/accounts/1/people/7/remove"`,
		`href="/founder/accounts"`} {
		if !strings.Contains(b, want) {
			t.Errorf("page is missing %q", want)
		}
	}
}

func TestLinkingAnEmail(t *testing.T) {
	rec := &linkRec{}
	rec.fake = fake{status: "live"}
	h := accountsHandler(rec)
	c := signIn(t, h)
	w := do(h, "POST", "/founder/accounts/1/people", url.Values{"email": {"new@acme.test"}, "role": {"admin"}}, c)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/founder/accounts" {
		t.Fatalf("%d %q", w.Code, w.Header().Get("Location"))
	}
	if len(rec.calls) != 1 || rec.calls[0] != "link 1 new@acme.test admin" || rec.by[0] != (store.Actor{Type: "system", Login: "founder"}) {
		t.Fatalf("calls = %v by %v", rec.calls, rec.by)
	}
	w = do(h, "POST", "/founder/accounts/1/people/7/remove", nil, c)
	if w.Code != http.StatusSeeOther || len(rec.calls) != 2 || rec.calls[1] != "unlink 1 7" {
		t.Fatalf("remove: %d %v", w.Code, rec.calls)
	}
}

func TestLinkingProblemsAreExplained(t *testing.T) {
	for _, c := range []struct {
		err  error
		code int
		text string
	}{
		{store.ErrBadEmail, 400, "not a valid email"},
		{store.ErrBadAccountRole, 400, "account role must be"},
		{store.ErrNotFound, 404, ""},
	} {
		rec := &linkRec{err: c.err}
		rec.fake = fake{status: "live"}
		h := accountsHandler(rec)
		w := do(h, "POST", "/founder/accounts/1/people", url.Values{"email": {"x"}, "role": {"owner"}}, signIn(t, h))
		if w.Code != c.code || !strings.Contains(strings.ToLower(w.Body.String()), c.text) {
			t.Errorf("%v: %d\n%s", c.err, w.Code, w.Body.String())
		}
	}
}

func TestAccountPostsNeedSignInAndSameOrigin(t *testing.T) {
	rec := &linkRec{}
	rec.fake = fake{status: "live"}
	h := accountsHandler(rec)
	form := url.Values{"email": {"new@acme.test"}, "role": {"owner"}}
	for _, p := range []string{"/founder/accounts/1/people", "/founder/accounts/1/people/7/remove"} {
		if w := do(h, "POST", p, form, nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/founder" {
			t.Errorf("signed out POST %s: %d", p, w.Code)
		}
		if w := postFrom(h, p, form, "https://evil.test", signIn(t, h)); w.Code != http.StatusForbidden {
			t.Errorf("cross-site POST %s: %d, want 403", p, w.Code)
		}
	}
	for _, p := range []string{"/founder/accounts/x/people", "/founder/accounts/1/people/x/remove", "/founder/accounts/0/people"} {
		if w := do(h, "POST", p, form, signIn(t, h)); w.Code != http.StatusNotFound {
			t.Errorf("POST %s: %d, want 404", p, w.Code)
		}
	}
	if len(rec.calls) != 0 {
		t.Errorf("refused posts reached the store: %v", rec.calls)
	}
}
