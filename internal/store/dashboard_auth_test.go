package store_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// signedIn links email to installation inst and signs it in to the dashboard.
func signedIn(t *testing.T, s *store.Store, inst int64, email, role string, now time.Time) store.DashboardSession {
	t.Helper()
	if _, err := s.LinkAccountMember(ctx, inst, email, role, founder); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := s.StartDashboardLogin(ctx, email, now)
	if err != nil || !ok {
		t.Fatalf("StartDashboardLogin = %v, %v", ok, err)
	}
	sess, err := s.FinishDashboardLogin(ctx, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestLinkAccountMembers(t *testing.T) {
	s := newStore(t)
	seedApp(t, s, 1, 10)

	m, err := s.LinkAccountMember(ctx, 1, " Ana@Example.com ", "owner", founder)
	if err != nil {
		t.Fatal(err)
	}
	if m.Email != "ana@example.com" || m.Role != "owner" || m.UserID == 0 {
		t.Fatalf("member = %+v", m)
	}
	again, err := s.LinkAccountMember(ctx, 1, "ana@example.com", "admin", founder)
	if err != nil || again.UserID != m.UserID || again.Role != "admin" {
		t.Fatalf("second link = %+v, %v; want same user with role admin", again, err)
	}
	if n := count(t, s, `SELECT count(*) FROM account_members`); n != 1 {
		t.Fatalf("%d link rows, want 1", n)
	}

	if _, err := s.LinkAccountMember(ctx, 1, "bob@example.com", "viewer", founder); !errors.Is(err, store.ErrBadAccountRole) {
		t.Errorf("role viewer: %v, want ErrBadAccountRole", err)
	}
	if _, err := s.LinkAccountMember(ctx, 1, "not an email", "member", founder); !errors.Is(err, store.ErrBadEmail) {
		t.Errorf("bad email: %v, want ErrBadEmail", err)
	}
	if _, err := s.LinkAccountMember(ctx, 999, "bob@example.com", "member", founder); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown account: %v, want ErrNotFound", err)
	}
	if n := count(t, s, `SELECT count(*) FROM audit_events WHERE action = 'account.member_added' AND installation_id = 1`); n != 2 {
		t.Errorf("%d audit rows, want 2 (one per successful link)", n)
	}

	list, err := s.ListAccountMembers(ctx, 1)
	if err != nil || len(list) != 1 || list[0].Email != "ana@example.com" || list[0].Role != "admin" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := s.UnlinkAccountMember(ctx, 1, m.UserID, founder); err != nil {
		t.Fatal(err)
	}
	if err := s.UnlinkAccountMember(ctx, 1, m.UserID, founder); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second unlink: %v, want ErrNotFound", err)
	}
	if n := count(t, s, `SELECT count(*) FROM audit_events WHERE action = 'account.member_removed' AND installation_id = 1`); n != 1 {
		t.Errorf("%d removal audit rows, want 1", n)
	}
}

func TestDashboardLoginRefusesStrangers(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	now := time.Now()
	// Being invited to an app, or a member of it, is not access to the dashboard.
	if _, err := s.CreateInvite(ctx, app.ID, "guest@example.com", "member", founder); err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"nobody@example.com", "not an email", "", "guest@example.com"} {
		raw, ok, err := s.StartDashboardLogin(ctx, email, now)
		if err != nil || ok || raw != "" {
			t.Errorf("StartDashboardLogin(%q) = %q, %v, %v; want nothing", email, raw, ok, err)
		}
	}
	if n := count(t, s, `SELECT count(*) FROM dashboard_login_tokens`); n != 0 {
		t.Errorf("%d tokens stored for strangers", n)
	}
}

func TestDashboardLoginFlow(t *testing.T) {
	s := newStore(t)
	seedApp(t, s, 1, 10)
	now := time.Now()
	sess := signedIn(t, s, 1, "ana@example.com", "owner", now)
	if sess.Token == "" || !sess.ExpiresAt.Equal(now.Add(store.DashboardSessionTTL)) {
		t.Fatalf("session = %+v", sess)
	}

	u, err := s.DashboardUserBySession(ctx, sess.Token, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "ana@example.com" || len(u.Accounts) != 1 || u.Accounts[0].Installation.ID != 1 || u.Accounts[0].Role != "owner" {
		t.Fatalf("user = %+v", u)
	}
	if n := count(t, s, `SELECT count(*) FROM users WHERE email = 'ana@example.com' AND last_login_at IS NOT NULL`); n != 1 {
		t.Error("last_login_at not set")
	}
	if n := count(t, s, `SELECT count(*) FROM audit_events WHERE action = 'dashboard.signed_in' AND installation_id = 1 AND actor_login = 'ana@example.com'`); n != 1 {
		t.Errorf("%d sign-in audit rows, want 1", n)
	}
}

func TestDashboardSecretsAreStoredHashed(t *testing.T) {
	s := newStore(t)
	seedApp(t, s, 1, 10)
	now := time.Now()
	if _, err := s.LinkAccountMember(ctx, 1, "ana@example.com", "owner", founder); err != nil {
		t.Fatal(err)
	}
	raw, _, _ := s.StartDashboardLogin(ctx, "ana@example.com", now)
	sess, err := s.FinishDashboardLogin(ctx, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	// What is stored is the SHA-256 of each secret (32 bytes), and no row holds the raw text.
	for table, v := range map[string]string{"dashboard_login_tokens": raw, "dashboard_sessions": sess.Token} {
		if n := count(t, s, `SELECT count(*) FROM `+table+` WHERE token_hash = $1 AND length(token_hash) = 32`, authn.Hash(v)); n != 1 {
			t.Errorf("%s: %d rows hold the hash of the secret, want 1", table, n)
		}
		if n := count(t, s, `SELECT count(*) FROM `+table+` WHERE token_hash = convert_to($1, 'UTF8')`, v); n != 0 {
			t.Errorf("%s: the raw secret is stored", table)
		}
	}
}

func TestDashboardTokenIsSingleUseAndExpires(t *testing.T) {
	s := newStore(t)
	seedApp(t, s, 1, 10)
	now := time.Now()
	if _, err := s.LinkAccountMember(ctx, 1, "ana@example.com", "owner", founder); err != nil {
		t.Fatal(err)
	}
	raw, _, _ := s.StartDashboardLogin(ctx, "ana@example.com", now)
	if _, err := s.FinishDashboardLogin(ctx, raw, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishDashboardLogin(ctx, raw, now); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("reused token: %v, want ErrInvalidToken", err)
	}
	old, _, _ := s.StartDashboardLogin(ctx, "ana@example.com", now)
	if _, err := s.FinishDashboardLogin(ctx, old, now.Add(store.LoginTokenTTL+time.Second)); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("expired token: %v, want ErrInvalidToken", err)
	}
	if _, err := s.FinishDashboardLogin(ctx, "garbage", now); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("unknown token: %v, want ErrInvalidToken", err)
	}
}

func TestDashboardConcurrentFinish(t *testing.T) {
	s := newStore(t)
	seedApp(t, s, 1, 10)
	now := time.Now()
	if _, err := s.LinkAccountMember(ctx, 1, "ana@example.com", "owner", founder); err != nil {
		t.Fatal(err)
	}
	raw, _, _ := s.StartDashboardLogin(ctx, "ana@example.com", now)
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.FinishDashboardLogin(ctx, raw, now)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, store.ErrInvalidToken):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || count(t, s, `SELECT count(*) FROM dashboard_sessions`) != 1 {
		t.Fatalf("%d logins succeeded, want exactly 1", ok)
	}
}

// Removing the link between sending the email and pressing Continue means no session.
func TestDashboardLoginFailsIfUnlinkedMeanwhile(t *testing.T) {
	s := newStore(t)
	seedApp(t, s, 1, 10)
	now := time.Now()
	m, _ := s.LinkAccountMember(ctx, 1, "ana@example.com", "owner", founder)
	raw, _, _ := s.StartDashboardLogin(ctx, "ana@example.com", now)
	if err := s.UnlinkAccountMember(ctx, 1, m.UserID, founder); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishDashboardLogin(ctx, raw, now); !errors.Is(err, store.ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestDashboardSessionRules(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	now := time.Now()
	sess := signedIn(t, s, 1, "ana@example.com", "owner", now)

	for name, c := range map[string]struct {
		token string
		at    time.Time
	}{
		"unknown":  {"nope", now},
		"empty":    {"", now},
		"expired":  {sess.Token, now.Add(store.DashboardSessionTTL + time.Second)},
		"a prefix": {sess.Token[:len(sess.Token)-1], now},
	} {
		if _, err := s.DashboardUserBySession(ctx, c.token, c.at); !errors.Is(err, store.ErrInvalidToken) {
			t.Errorf("%s: %v, want ErrInvalidToken", name, err)
		}
	}

	// An app visitor's session value is not a dashboard session.
	appSess := invitedLogin(t, s, app, "guest@example.com", now)
	if _, err := s.DashboardUserBySession(ctx, appSess.ID, now); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("app session accepted by the dashboard: %v", err)
	}

	if err := s.RevokeDashboardSession(ctx, sess.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DashboardUserBySession(ctx, sess.Token, now); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("revoked session: %v, want ErrInvalidToken", err)
	}
	if err := s.RevokeDashboardSession(ctx, "unknown"); err != nil {
		t.Errorf("revoking an unknown session: %v, want nil", err)
	}
}

func TestDashboardAccountsFollowTheLinks(t *testing.T) {
	s := newStore(t)
	seedApp(t, s, 1, 10)
	seedApp(t, s, 2, 20)
	now := time.Now()
	sess := signedIn(t, s, 1, "ana@example.com", "owner", now)

	u, _ := s.DashboardUserBySession(ctx, sess.Token, now)
	if len(u.Accounts) != 1 || u.Accounts[0].Installation.ID != 1 {
		t.Fatalf("accounts = %+v, want only account 1", u.Accounts)
	}
	m2, err := s.LinkAccountMember(ctx, 2, "ana@example.com", "member", founder)
	if err != nil {
		t.Fatal(err)
	}
	u, _ = s.DashboardUserBySession(ctx, sess.Token, now)
	roles := map[int64]string{}
	for _, a := range u.Accounts {
		roles[a.Installation.ID] = a.Role
	}
	if len(roles) != 2 || roles[1] != "owner" || roles[2] != "member" {
		t.Fatalf("roles = %v", roles)
	}
	// Another person's links do not leak in.
	other := signedIn(t, s, 2, "bob@example.com", "owner", now)
	if ou, _ := s.DashboardUserBySession(ctx, other.Token, now); len(ou.Accounts) != 1 || ou.Accounts[0].Installation.ID != 2 {
		t.Fatalf("bob's accounts = %+v", ou.Accounts)
	}

	// Dropping one account keeps the session; dropping the last one ends it at once.
	if err := s.UnlinkAccountMember(ctx, 2, m2.UserID, founder); err != nil {
		t.Fatal(err)
	}
	if u, err = s.DashboardUserBySession(ctx, sess.Token, now); err != nil || len(u.Accounts) != 1 {
		t.Fatalf("after one unlink: %+v, %v", u, err)
	}
	if err := s.UnlinkAccountMember(ctx, 1, u.ID, founder); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DashboardUserBySession(ctx, sess.Token, now); !errors.Is(err, store.ErrInvalidToken) {
		t.Fatalf("after the last unlink: %v, want ErrInvalidToken", err)
	}
	if n := count(t, s, `SELECT count(*) FROM dashboard_sessions WHERE revoked_at IS NULL AND user_id = $1`, u.ID); n != 0 {
		t.Errorf("%d sessions left open", n)
	}
}

func TestPruneRemovesOldDashboardRows(t *testing.T) {
	s := newStore(t)
	seedApp(t, s, 1, 10)
	now := time.Now()
	signedIn(t, s, 1, "ana@example.com", "owner", now.Add(-10*24*time.Hour))
	fresh := signedIn(t, s, 1, "bob@example.com", "owner", now)
	if _, err := s.Prune(ctx, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DashboardUserBySession(ctx, fresh.Token, now); err != nil {
		t.Errorf("live session was pruned: %v", err)
	}
	if n := count(t, s, `SELECT count(*) FROM dashboard_sessions`); n != 1 {
		t.Errorf("%d sessions left, want only the fresh one", n)
	}
	if n := count(t, s, `SELECT count(*) FROM dashboard_login_tokens`); n != 1 {
		t.Errorf("%d login tokens left, want only the fresh one", n)
	}
}
