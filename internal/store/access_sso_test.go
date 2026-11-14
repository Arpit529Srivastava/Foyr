package store_test

import (
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

func appAccess(t *testing.T, s *store.Store, slug string) authn.AppAccess {
	t.Helper()
	snap, err := s.Access(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range snap.Apps {
		if a.Slug == slug {
			return a
		}
	}
	t.Fatalf("app %s not in snapshot", slug)
	return authn.AppAccess{}
}

func TestAccessSnapshotIncludesAccountAdmins(t *testing.T) {
	s := newStore(t)
	crm := seedApp(t, s, 1, 10)
	blog := seedApp(t, s, 2, 11)
	now := time.Now()
	owner := dashUser(t, s, 1, "owner@x.test", "owner", now)
	admin := dashUser(t, s, 1, "admin@x.test", "admin", now)
	member := dashUser(t, s, 1, "member@x.test", "member", now)

	got := appAccess(t, s, crm.Slug).Members
	if got[owner] != "owner" || got[admin] != "admin" {
		t.Errorf("crm members = %v, want owner and admin with their roles", got)
	}
	if _, ok := got[member]; ok {
		t.Errorf("crm members = %v, a read-only account member must not be listed", got)
	}
	other := appAccess(t, s, blog.Slug).Members
	for _, uid := range []int64{owner, admin, member} {
		if _, ok := other[uid]; ok {
			t.Errorf("blog (another account) lists user %d: %v", uid, other)
		}
	}

	if err := s.UnlinkAccountMember(ctx, 1, admin, founder); err != nil {
		t.Fatal(err)
	}
	if _, ok := appAccess(t, s, crm.Slug).Members[admin]; ok {
		t.Error("unlinked admin is still in the snapshot")
	}

	if err := s.SyncInstallation(ctx, store.InstallationChange{Action: "suspend", InstallationID: 1, AccountLogin: "octo-dev", Actor: founder}); err != nil {
		t.Fatal(err)
	}
	if _, ok := appAccess(t, s, crm.Slug).Members[owner]; ok {
		t.Error("owner of a suspended account is still in the snapshot")
	}
}

func TestAccessSnapshotKeepsExplicitAppRole(t *testing.T) {
	s := newStore(t)
	crm := seedApp(t, s, 1, 10)
	now := time.Now()
	sess := invitedLogin(t, s, crm, "ana@example.com", now) // member of the app
	if _, err := s.CreateInvite(ctx, crm.ID, "viewer@example.com", "viewer", founder); err != nil {
		t.Fatal(err)
	}
	vraw, _, _ := s.StartLogin(ctx, crm.Slug, "viewer@example.com", now)
	vcode, _, err := s.FinishLogin(ctx, vraw, now)
	if err != nil {
		t.Fatal(err)
	}
	vsess, err := s.ExchangeCode(ctx, vcode, crm.Slug, now)
	if err != nil {
		t.Fatal(err)
	}
	// Both become owners of the account: their explicit app roles still win.
	for _, email := range []string{"ana@example.com", "viewer@example.com"} {
		if _, err := s.LinkAccountMember(ctx, 1, email, "owner", founder); err != nil {
			t.Fatal(err)
		}
	}
	got := appAccess(t, s, crm.Slug).Members
	if got[sess.UserID] != "member" || got[vsess.UserID] != "viewer" {
		t.Errorf("members = %v, want explicit roles member and viewer kept", got)
	}
}
