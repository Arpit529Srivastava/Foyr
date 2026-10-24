package store_test

import (
	"errors"
	"testing"

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
