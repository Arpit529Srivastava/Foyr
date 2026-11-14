package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// dashUser links email to installation inst with role, signs it in to the dashboard, and returns the user's id.
func dashUser(t *testing.T, s *store.Store, inst int64, email, role string, now time.Time) int64 {
	t.Helper()
	sess := signedIn(t, s, inst, email, role, now)
	u, err := s.DashboardUserBySession(ctx, sess.Token, now)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestIssueSSOCode(t *testing.T) {
	s := newStore(t)
	crm := seedApp(t, s, 1, 10)
	seedApp(t, s, 2, 11)
	now := time.Now()

	for _, c := range []struct{ email, role string }{{"owner@x.test", "owner"}, {"admin@x.test", "admin"}} {
		uid := dashUser(t, s, 1, c.email, c.role, now)
		code, err := s.IssueSSOCode(ctx, uid, crm.Slug, now)
		if err != nil || code == "" {
			t.Fatalf("%s: IssueSSOCode = %q, %v", c.role, code, err)
		}
		sess, err := s.ExchangeCode(ctx, code, crm.Slug, now)
		if err != nil {
			t.Fatalf("%s: ExchangeCode: %v", c.role, err)
		}
		if sess.UserID != uid || sess.Email != c.email || sess.AppID != crm.ID || sess.PublicID == "" {
			t.Errorf("%s: session = %+v, want user %d, %s, app %d with a public id", c.role, sess, uid, c.email, crm.ID)
		}
	}
}

func TestIssueSSORefusals(t *testing.T) {
	now := time.Now()
	setup := func(t *testing.T) (*store.Store, store.App) {
		s := newStore(t)
		crm := seedApp(t, s, 1, 10)
		seedApp(t, s, 2, 11)
		return s, crm
	}
	cases := []struct {
		name string
		run  func(t *testing.T, s *store.Store, crm store.App) (int64, string)
	}{
		{"member role", func(t *testing.T, s *store.Store, crm store.App) (int64, string) {
			return dashUser(t, s, 1, "m@x.test", "member", now), crm.Slug
		}},
		{"owner of another account", func(t *testing.T, s *store.Store, crm store.App) (int64, string) {
			return dashUser(t, s, 2, "o2@x.test", "owner", now), crm.Slug
		}},
		{"unknown app", func(t *testing.T, s *store.Store, crm store.App) (int64, string) {
			return dashUser(t, s, 1, "o@x.test", "owner", now), "no-such-app"
		}},
		{"suspended installation", func(t *testing.T, s *store.Store, crm store.App) (int64, string) {
			uid := dashUser(t, s, 1, "o@x.test", "owner", now)
			if err := s.SyncInstallation(ctx, store.InstallationChange{Action: "suspend", InstallationID: 1, AccountLogin: "octo-dev", Actor: founder}); err != nil {
				t.Fatal(err)
			}
			return uid, crm.Slug
		}},
		{"removed installation", func(t *testing.T, s *store.Store, crm store.App) (int64, string) {
			uid := dashUser(t, s, 1, "o@x.test", "owner", now)
			if err := s.SyncInstallation(ctx, store.InstallationChange{Action: "deleted", InstallationID: 1, AccountLogin: "octo-dev", Actor: founder}); err != nil {
				t.Fatal(err)
			}
			return uid, crm.Slug
		}},
		{"unlinked after sign-in", func(t *testing.T, s *store.Store, crm store.App) (int64, string) {
			uid := dashUser(t, s, 1, "o@x.test", "owner", now)
			if err := s.UnlinkAccountMember(ctx, 1, uid, founder); err != nil {
				t.Fatal(err)
			}
			return uid, crm.Slug
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, crm := setup(t)
			uid, slug := c.run(t, s, crm)
			code, err := s.IssueSSOCode(ctx, uid, slug, now)
			if !errors.Is(err, store.ErrNoSSO) || code != "" {
				t.Fatalf("IssueSSOCode = %q, %v; want ErrNoSSO and no code", code, err)
			}
			if n := count(t, s, `SELECT count(*) FROM auth_codes`); n != 0 {
				t.Errorf("%d auth_codes rows, want 0", n)
			}
			if n := count(t, s, `SELECT count(*) FROM audit_events WHERE details->>'method' = 'sso'`); n != 0 {
				t.Errorf("%d sso audit rows, want 0", n)
			}
		})
	}
}

func TestSSOCodeSingleUseAndExpiry(t *testing.T) {
	s := newStore(t)
	crm := seedApp(t, s, 1, 10)
	now := time.Now()
	uid := dashUser(t, s, 1, "o@x.test", "owner", now)

	code, err := s.IssueSSOCode(ctx, uid, crm.Slug, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExchangeCode(ctx, code, crm.Slug, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExchangeCode(ctx, code, crm.Slug, now); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("second ExchangeCode: %v, want ErrInvalidToken", err)
	}

	old, err := s.IssueSSOCode(ctx, uid, crm.Slug, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExchangeCode(ctx, old, crm.Slug, now.Add(store.AuthCodeTTL+time.Second)); !errors.Is(err, store.ErrInvalidToken) {
		t.Errorf("expired ExchangeCode: %v, want ErrInvalidToken", err)
	}
}

func TestSSOCodeStoredHashedAndAudited(t *testing.T) {
	s := newStore(t)
	crm := seedApp(t, s, 1, 10)
	now := time.Now()
	uid := dashUser(t, s, 1, "o@x.test", "owner", now)

	code, err := s.IssueSSOCode(ctx, uid, crm.Slug, now)
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, `SELECT count(*) FROM auth_codes t WHERE row_to_json(t)::text LIKE '%' || $1 || '%'`, code); n != 0 {
		t.Error("auth_codes stores the raw SSO code")
	}
	if n := count(t, s, `SELECT count(*) FROM audit_events WHERE action = 'user.signed_in' AND details->>'method' = 'sso' AND installation_id = 1 AND app_id = $1`, crm.ID); n != 1 {
		t.Errorf("%d sso audit rows, want 1", n)
	}
}
