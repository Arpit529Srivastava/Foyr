package store_test

import (
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// seedApp creates a private app owned by installation inst through the push path.
func seedApp(t *testing.T, s *store.Store, inst, repo int64) store.App {
	t.Helper()
	if _, _, err := s.CreateDeployment(ctx, push(inst, repo, sha(int(repo)))); err != nil {
		t.Fatal(err)
	}
	apps, err := s.ListApps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range apps {
		if a.GitHubRepoID == repo {
			return a
		}
	}
	t.Fatalf("app for repo %d not found", repo)
	return store.App{}
}

var founder = store.Actor{Type: "system", Login: "founder"}

func TestInvites(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)

	inv, err := s.CreateInvite(ctx, app.ID, " Ana@Example.com ", "member", founder)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Email != "ana@example.com" || inv.Role != "member" || inv.InstallationID != 1 || inv.InvitedBy != "founder" {
		t.Fatalf("invite = %+v", inv)
	}

	again, err := s.CreateInvite(ctx, app.ID, "ana@example.com", "admin", founder)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != inv.ID || again.Role != "admin" {
		t.Fatalf("second invite = %+v, want same row %d with role admin", again, inv.ID)
	}
	if n := count(t, s, `SELECT count(*) FROM invites`); n != 1 {
		t.Fatalf("%d invite rows, want 1", n)
	}

	if _, err := s.CreateInvite(ctx, app.ID, "bob@example.com", "god", founder); !errors.Is(err, store.ErrBadRole) {
		t.Errorf("role god: err = %v, want ErrBadRole", err)
	}
	if _, err := s.CreateInvite(ctx, app.ID, "not an email", "member", founder); !errors.Is(err, store.ErrBadEmail) {
		t.Errorf("bad email: err = %v, want ErrBadEmail", err)
	}
	if _, err := s.CreateInvite(ctx, 9999, "bob@example.com", "member", founder); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown app: err = %v, want ErrNotFound", err)
	}

	if n := count(t, s, `SELECT count(*) FROM audit_events WHERE action = 'invite.created' AND installation_id = 1 AND app_id = $1`, app.ID); n != 2 {
		t.Errorf("%d invite.created audit rows, want 2 (one per successful call)", n)
	}

	list, err := s.ListInvites(ctx, app.ID)
	if err != nil || len(list) != 1 || list[0].ID != inv.ID {
		t.Fatalf("ListInvites = %+v, %v", list, err)
	}
	if err := s.RevokeInvite(ctx, inv.ID, founder); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListInvites(ctx, app.ID); len(list) != 0 {
		t.Errorf("revoked invite still listed: %+v", list)
	}
	if n := count(t, s, `SELECT count(*) FROM audit_events WHERE action = 'invite.revoked' AND installation_id = 1`); n != 1 {
		t.Errorf("%d invite.revoked audit rows, want 1", n)
	}
	if err := s.RevokeInvite(ctx, inv.ID, founder); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second revoke: err = %v, want ErrNotFound", err)
	}
}

func TestInviteForCurrentMemberRefused(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	var uid int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('ana@example.com') RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO app_members (app_id, user_id, installation_id, role) VALUES ($1, $2, 1, 'member')`, app.ID, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateInvite(ctx, app.ID, "Ana@example.com", "admin", founder); !errors.Is(err, store.ErrAlreadyMember) {
		t.Fatalf("err = %v, want ErrAlreadyMember", err)
	}
	members, err := s.ListMembers(ctx, app.ID)
	if err != nil || len(members) != 1 || members[0].Email != "ana@example.com" || members[0].Role != "member" {
		t.Fatalf("ListMembers = %+v, %v", members, err)
	}
}

func TestRemoveMember(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	var uid int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('ana@example.com') RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO app_members (app_id, user_id, installation_id, role) VALUES ($1, $2, 1, 'member')`,
		`INSERT INTO sessions (id, user_id, app_id, installation_id, expires_at) VALUES ('sess-1', $2, $1, 1, now() + interval '1 day')`,
	} {
		if _, err := s.Pool.Exec(ctx, q, app.ID, uid); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RemoveMember(ctx, app.ID, uid, founder); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, `SELECT count(*) FROM app_members`); n != 0 {
		t.Errorf("%d members left", n)
	}
	if n := count(t, s, `SELECT count(*) FROM sessions WHERE id = 'sess-1' AND revoked_at IS NOT NULL`); n != 1 {
		t.Error("the member's session was not revoked")
	}
	if n := count(t, s, `SELECT count(*) FROM audit_events WHERE action = 'member.removed' AND installation_id = 1`); n != 1 {
		t.Errorf("%d member.removed audit rows, want 1", n)
	}
	if err := s.RemoveMember(ctx, app.ID, uid, founder); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("second removal: err = %v, want ErrNotFound", err)
	}
}

func TestSetAccessMode(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	if app.AccessMode != "private" {
		t.Fatalf("new app mode = %q, want private", app.AccessMode)
	}
	if err := s.SetAccessMode(ctx, app.ID, "secret", founder); !errors.Is(err, store.ErrBadMode) {
		t.Errorf("mode secret: err = %v, want ErrBadMode", err)
	}
	if err := s.SetAccessMode(ctx, app.ID, "public", founder); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetApp(ctx, app.ID)
	if err != nil || got.AccessMode != "public" {
		t.Fatalf("after set: %+v, %v", got, err)
	}
	if n := count(t, s, `SELECT count(*) FROM audit_events WHERE action = 'app.access_changed' AND installation_id = 1 AND app_id = $1`, app.ID); n != 1 {
		t.Errorf("%d app.access_changed audit rows, want 1", n)
	}
	if err := s.SetAccessMode(ctx, 9999, "public", founder); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown app: err = %v, want ErrNotFound", err)
	}
}

func TestAppBySlug(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	got, err := s.AppBySlug(ctx, app.Slug)
	if err != nil || got.ID != app.ID {
		t.Fatalf("AppBySlug(%q) = %+v, %v", app.Slug, got, err)
	}
	if _, err := s.AppBySlug(ctx, "nope-nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown slug: err = %v, want ErrNotFound", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE apps SET removed_at = now() WHERE id = $1`, app.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppBySlug(ctx, app.Slug); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("removed app: err = %v, want ErrNotFound", err)
	}
}

// invitedLogin invites email to app and runs the whole login up to a session.
func invitedLogin(t *testing.T, s *store.Store, app store.App, email string, now time.Time) store.Session {
	t.Helper()
	if _, err := s.CreateInvite(ctx, app.ID, email, "member", founder); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := s.StartLogin(ctx, app.Slug, email, now)
	if err != nil || !ok {
		t.Fatalf("StartLogin = %v, %v", ok, err)
	}
	code, _, err := s.FinishLogin(ctx, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.ExchangeCode(ctx, code, app.Slug, now)
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestLoginFlow(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	now := time.Now()
	if _, err := s.CreateInvite(ctx, app.ID, "ana@example.com", "admin", founder); err != nil {
		t.Fatal(err)
	}

	raw, ok, err := s.StartLogin(ctx, app.Slug, "ANA@example.com", now)
	if err != nil || !ok || raw == "" {
		t.Fatalf("StartLogin = %q, %v, %v", raw, ok, err)
	}
	code, slug, err := s.FinishLogin(ctx, raw, now.Add(time.Minute))
	if err != nil || code == "" || slug != app.Slug {
		t.Fatalf("FinishLogin = %q, %q, %v", code, slug, err)
	}

	members, _ := s.ListMembers(ctx, app.ID)
	if len(members) != 1 || members[0].Email != "ana@example.com" || members[0].Role != "admin" {
		t.Fatalf("members = %+v, want Ana as admin", members)
	}
	if open, _ := s.ListInvites(ctx, app.ID); len(open) != 0 {
		t.Errorf("invite still open after login: %+v", open)
	}
	if n := count(t, s, `SELECT count(*) FROM audit_events WHERE action = 'user.signed_in' AND installation_id = 1 AND actor_type = 'foyr_user'`); n != 1 {
		t.Errorf("%d user.signed_in audit rows, want 1", n)
	}
	if n := count(t, s, `SELECT count(*) FROM users WHERE email = 'ana@example.com' AND last_login_at IS NOT NULL`); n != 1 {
		t.Errorf("user row or last_login_at missing")
	}

	sess, err := s.ExchangeCode(ctx, code, app.Slug, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if sess.UserID != members[0].UserID || sess.Email != "ana@example.com" || sess.Slug != app.Slug ||
		sess.AppID != app.ID || sess.InstallationID != 1 || sess.ID == "" {
		t.Fatalf("session = %+v", sess)
	}
	if want := now.Add(time.Minute).Add(7 * 24 * time.Hour); sess.ExpiresAt.Sub(want).Abs() > time.Second {
		t.Errorf("ExpiresAt = %v, want %v", sess.ExpiresAt, want)
	}
}

func TestLoginRefusesStrangers(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	now := time.Now()

	raw, ok, err := s.StartLogin(ctx, app.Slug, "stranger@example.com", now)
	if err != nil || ok || raw != "" {
		t.Fatalf("private app, stranger: %q, %v, %v; want not ok", raw, ok, err)
	}
	if _, ok, _ := s.StartLogin(ctx, "no-such-app", "stranger@example.com", now); ok {
		t.Error("unknown app gave ok")
	}
	if _, ok, _ := s.StartLogin(ctx, app.Slug, "not an email", now); ok {
		t.Error("bad email gave ok")
	}
	if n := count(t, s, `SELECT count(*) FROM login_tokens`); n != 0 {
		t.Fatalf("%d tokens stored for refused logins, want 0", n)
	}

	if err := s.SetAccessMode(ctx, app.ID, "public", founder); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.StartLogin(ctx, app.Slug, "stranger@example.com", now); err != nil || !ok {
		t.Errorf("public app, stranger: ok = %v, err = %v; want ok", ok, err)
	}
}

func TestTokensAreSingleUse(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	other := seedApp(t, s, 1, 11)
	now := time.Now()
	start := func() string {
		t.Helper()
		if _, err := s.CreateInvite(ctx, app.ID, "ana@example.com", "member", founder); err != nil && !errors.Is(err, store.ErrAlreadyMember) {
			t.Fatal(err)
		}
		raw, ok, err := s.StartLogin(ctx, app.Slug, "ana@example.com", now)
		if err != nil || !ok {
			t.Fatalf("StartLogin = %v, %v", ok, err)
		}
		return raw
	}

	raw := start()
	code, _, err := s.FinishLogin(ctx, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.FinishLogin(ctx, raw, now); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("second FinishLogin: err = %v, want ErrInvalidToken", err)
	}
	if _, _, err := s.FinishLogin(ctx, "unknown", now); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("unknown token: err = %v, want ErrInvalidToken", err)
	}
	if _, err := s.ExchangeCode(ctx, code, other.Slug, now); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("code for another app: err = %v, want ErrInvalidToken", err)
	}
	if _, err := s.ExchangeCode(ctx, code, app.Slug, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExchangeCode(ctx, code, app.Slug, now); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("second ExchangeCode: err = %v, want ErrInvalidToken", err)
	}

	late := start()
	if _, _, err := s.FinishLogin(ctx, late, now.Add(16*time.Minute)); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("token used after 16 minutes: err = %v, want ErrInvalidToken", err)
	}
	code2, _, err := s.FinishLogin(ctx, start(), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExchangeCode(ctx, code2, app.Slug, now.Add(61*time.Second)); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("code used after 61 seconds: err = %v, want ErrInvalidToken", err)
	}
}

func TestTokensStoredHashed(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	now := time.Now()
	if _, err := s.CreateInvite(ctx, app.ID, "ana@example.com", "member", founder); err != nil {
		t.Fatal(err)
	}
	raw, _, _ := s.StartLogin(ctx, app.Slug, "ana@example.com", now)
	code, _, err := s.FinishLogin(ctx, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.ExchangeCode(ctx, code, app.Slug, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ table, secret string }{{"login_tokens", raw}, {"auth_codes", code}} {
		if n := count(t, s, `SELECT count(*) FROM `+c.table+` t WHERE row_to_json(t)::text LIKE '%' || $1 || '%'`, c.secret); n != 0 {
			t.Errorf("%s stores the raw secret", c.table)
		}
	}
	if sess.ID == raw || sess.ID == code {
		t.Error("session id reuses a one-time secret")
	}
}

func TestAccessSnapshot(t *testing.T) {
	s := newStore(t)
	crm := seedApp(t, s, 1, 10)
	blog := seedApp(t, s, 2, 11)
	if err := s.SetAccessMode(ctx, blog.ID, "public", founder); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	sess := invitedLogin(t, s, crm, "ana@example.com", now)

	snap, err := s.Access(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byslug := map[string]string{}
	for _, a := range snap.Apps {
		byslug[a.Slug] = a.Mode
		if a.Slug == crm.Slug && a.Members[sess.UserID] != "member" {
			t.Errorf("crm members = %v, want Ana as member", a.Members)
		}
	}
	if byslug[crm.Slug] != "private" || byslug[blog.Slug] != "public" || len(snap.Apps) != 2 {
		t.Errorf("modes = %v", byslug)
	}
	if len(snap.Revoked) != 0 {
		t.Errorf("revoked = %v, want none yet", snap.Revoked)
	}

	if err := s.RemoveMember(ctx, crm.ID, sess.UserID, founder); err != nil {
		t.Fatal(err)
	}
	snap, _ = s.Access(ctx)
	for _, a := range snap.Apps {
		if _, ok := a.Members[sess.UserID]; ok {
			t.Errorf("%s still lists the removed user", a.Slug)
		}
	}
	if len(snap.Revoked) != 1 || snap.Revoked[0] != sess.ID {
		t.Errorf("revoked = %v, want [%s]", snap.Revoked, sess.ID)
	}

	if err := s.RevokeSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentFinishLogin(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	now := time.Now()
	if _, err := s.CreateInvite(ctx, app.ID, "ana@example.com", "member", founder); err != nil {
		t.Fatal(err)
	}
	raw, _, _ := s.StartLogin(ctx, app.Slug, "ana@example.com", now)

	var wg sync.WaitGroup
	results := make(chan error, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.FinishLogin(ctx, raw, now)
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
	if ok != 1 {
		t.Fatalf("%d concurrent logins succeeded, want exactly 1", ok)
	}
	if n := count(t, s, `SELECT count(*) FROM auth_codes`); n != 1 {
		t.Errorf("%d codes issued, want 1", n)
	}
}

func TestPruneRemovesOldAuthRows(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	now := time.Now()
	sess := invitedLogin(t, s, app, "ana@example.com", now.Add(-10*24*time.Hour))
	fresh := invitedLogin(t, s, app, "bob@example.com", now)

	if _, err := s.Prune(ctx, now); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, `SELECT count(*) FROM sessions WHERE id = $1`, sess.ID); n != 0 {
		t.Error("expired session not pruned")
	}
	if n := count(t, s, `SELECT count(*) FROM sessions WHERE id = $1`, fresh.ID); n != 1 {
		t.Error("live session was pruned")
	}
	if n := count(t, s, `SELECT count(*) FROM login_tokens`); n != 1 {
		t.Errorf("%d login tokens left, want only the fresh one", n)
	}
	if n := count(t, s, `SELECT count(*) FROM auth_codes`); n != 1 {
		t.Errorf("%d auth codes left, want only the fresh one", n)
	}
}

func TestExchangeCodeReturnsThePublicID(t *testing.T) {
	s := newStore(t)
	app := seedApp(t, s, 1, 10)
	sess := invitedLogin(t, s, app, "ana@example.com", time.Now())
	if !regexp.MustCompile(`^usr_[0-9a-f]{16}$`).MatchString(sess.PublicID) {
		t.Fatalf("PublicID = %q, want usr_ plus 16 hex", sess.PublicID)
	}
	var want string
	if err := s.Pool.QueryRow(ctx, `SELECT public_id FROM users WHERE id = $1`, sess.UserID).Scan(&want); err != nil {
		t.Fatal(err)
	}
	if sess.PublicID != want {
		t.Errorf("PublicID = %q, users row has %q", sess.PublicID, want)
	}
}
