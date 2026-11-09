package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
)

// ErrBadAccountRole is returned for an account role other than owner, admin or member.
var ErrBadAccountRole = errors.New("account role must be owner, admin or member")

// DashboardSessionTTL is how long a dashboard sign-in lasts.
const DashboardSessionTTL = SessionTTL

// AccountMember is a person linked to a GitHub account, so they may open its dashboard.
type AccountMember struct {
	UserID    int64
	Email     string
	Role      string
	CreatedAt time.Time
}

// AccountAccess is one account a signed-in person may open, with their role in it.
type AccountAccess struct {
	Installation Installation
	Role         string
}

// DashboardUser is a signed-in dashboard person and the accounts they may open.
type DashboardUser struct {
	ID       int64
	Email    string
	Accounts []AccountAccess
}

// DashboardSession is a new dashboard sign-in; Token is the cookie value and is never stored.
type DashboardSession struct {
	Token     string
	ExpiresAt time.Time
}

// LinkAccountMember lets email open the account's dashboard with role, creating the user if new.
// Linking again changes the role.
func (s *Store) LinkAccountMember(ctx context.Context, installationID int64, email, role string, by Actor) (AccountMember, error) {
	email, ok := authn.NormalizeEmail(email)
	if !ok {
		return AccountMember{}, ErrBadEmail
	}
	if role != "owner" && role != "admin" && role != "member" {
		return AccountMember{}, ErrBadAccountRole
	}
	var m AccountMember
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM installations WHERE id = $1 AND removed_at IS NULL)`, installationID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		if err := tx.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1)
			ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email RETURNING id`, email).Scan(&m.UserID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO account_members (installation_id, user_id, role) VALUES ($1, $2, $3)
			ON CONFLICT (installation_id, user_id) DO UPDATE SET role = EXCLUDED.role
			RETURNING created_at`, installationID, m.UserID, role).Scan(&m.CreatedAt); err != nil {
			return err
		}
		m.Email, m.Role = email, role
		return audit(ctx, tx, AuditEvent{InstallationID: installationID, Actor: by, Action: "account.member_added",
			TargetType: "user", TargetID: itoa(m.UserID), Details: map[string]any{"email": email, "role": role}})
	})
	return m, err
}

// UnlinkAccountMember takes a person off an account. When it was their last account, their sessions end.
func (s *Store) UnlinkAccountMember(ctx context.Context, installationID, userID int64, by Actor) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var email string
		if err := tx.QueryRow(ctx, `DELETE FROM account_members m USING users u
			WHERE m.installation_id = $1 AND m.user_id = $2 AND u.id = m.user_id RETURNING u.email`,
			installationID, userID).Scan(&email); err != nil {
			return notFound(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE dashboard_sessions SET revoked_at = now()
			WHERE user_id = $1 AND revoked_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM account_members WHERE user_id = $1)`, userID); err != nil {
			return err
		}
		return audit(ctx, tx, AuditEvent{InstallationID: installationID, Actor: by, Action: "account.member_removed",
			TargetType: "user", TargetID: itoa(userID), Details: map[string]any{"email": email}})
	})
}

// ListAccountMembers returns who may open the account's dashboard, oldest first.
func (s *Store) ListAccountMembers(ctx context.Context, installationID int64) ([]AccountMember, error) {
	rows, err := s.Pool.Query(ctx, `SELECT m.user_id, u.email, m.role, m.created_at
		FROM account_members m JOIN users u ON u.id = m.user_id WHERE m.installation_id = $1
		ORDER BY m.created_at, m.user_id`, installationID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (AccountMember, error) {
		var m AccountMember
		err := r.Scan(&m.UserID, &m.Email, &m.Role, &m.CreatedAt)
		return m, err
	})
}

// StartDashboardLogin stores a login token for email and returns its raw value, but only when the email is
// linked to at least one account. ok is false (and nothing is stored) otherwise.
func (s *Store) StartDashboardLogin(ctx context.Context, email string, now time.Time) (raw string, ok bool, err error) {
	email, valid := authn.NormalizeEmail(email)
	if !valid {
		return "", false, nil
	}
	var linked bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_members m JOIN users u ON u.id = m.user_id
		JOIN installations i ON i.id = m.installation_id WHERE u.email = $1 AND i.removed_at IS NULL)`, email).Scan(&linked); err != nil {
		return "", false, err
	}
	if !linked {
		return "", false, nil
	}
	raw, hash := authn.NewSecret()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO dashboard_login_tokens (token_hash, email, expires_at, created_at)
		VALUES ($1, $2, $3, $4)`, hash, email, now.Add(LoginTokenTTL), now); err != nil {
		return "", false, err
	}
	return raw, true, nil
}

// FinishDashboardLogin spends a login token and starts a session. It works once, and only while the email
// is still linked to an account.
func (s *Store) FinishDashboardLogin(ctx context.Context, raw string, now time.Time) (DashboardSession, error) {
	var sess DashboardSession
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var email string
		if err := tx.QueryRow(ctx, `UPDATE dashboard_login_tokens SET used_at = $2
			WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2 RETURNING email`,
			authn.Hash(raw), now).Scan(&email); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidToken
			}
			return err
		}
		var userID int64
		if err := tx.QueryRow(ctx, `UPDATE users SET last_login_at = $2 WHERE email = $1
			AND EXISTS (SELECT 1 FROM account_members WHERE user_id = users.id) RETURNING id`, email, now).Scan(&userID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidToken
			}
			return err
		}
		var hash []byte
		sess.Token, hash = authn.NewSecret()
		sess.ExpiresAt = now.Add(DashboardSessionTTL)
		if _, err := tx.Exec(ctx, `INSERT INTO dashboard_sessions (token_hash, user_id, created_at, expires_at)
			VALUES ($1, $2, $3, $4)`, hash, userID, now, sess.ExpiresAt); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT installation_id FROM account_members WHERE user_id = $1 ORDER BY installation_id`, userID)
		if err != nil {
			return err
		}
		insts, err := pgx.CollectRows(rows, pgx.RowTo[int64])
		if err != nil {
			return err
		}
		for _, inst := range insts {
			if err := audit(ctx, tx, AuditEvent{InstallationID: inst, Actor: Actor{Type: "foyr_user", ID: itoa(userID), Login: email},
				Action: "dashboard.signed_in", TargetType: "user", TargetID: itoa(userID)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return DashboardSession{}, err
	}
	return sess, nil
}

// DashboardUserBySession finds who a session cookie value belongs to, with the accounts they may open now.
// Unknown, expired, revoked and account-less sessions are all ErrInvalidToken.
func (s *Store) DashboardUserBySession(ctx context.Context, raw string, now time.Time) (DashboardUser, error) {
	var u DashboardUser
	if raw == "" {
		return u, ErrInvalidToken
	}
	if err := s.Pool.QueryRow(ctx, `SELECT u.id, u.email FROM dashboard_sessions d JOIN users u ON u.id = d.user_id
		WHERE d.token_hash = $1 AND d.revoked_at IS NULL AND d.expires_at > $2`, authn.Hash(raw), now).Scan(&u.ID, &u.Email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return u, ErrInvalidToken
		}
		return u, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT i.id, i.account_login, i.suspended_at IS NOT NULL, i.created_at, m.role
		FROM account_members m JOIN installations i ON i.id = m.installation_id
		WHERE m.user_id = $1 AND i.removed_at IS NULL ORDER BY lower(i.account_login), i.id`, u.ID)
	if err != nil {
		return u, err
	}
	u.Accounts, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (AccountAccess, error) {
		var a AccountAccess
		err := r.Scan(&a.Installation.ID, &a.Installation.Login, &a.Installation.Suspended, &a.Installation.CreatedAt, &a.Role)
		return a, err
	})
	if err != nil {
		return u, err
	}
	if len(u.Accounts) == 0 {
		return DashboardUser{}, ErrInvalidToken
	}
	return u, nil
}

// RevokeDashboardSession ends one session (sign out). An unknown or ended session is not an error.
func (s *Store) RevokeDashboardSession(ctx context.Context, raw string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE dashboard_sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, authn.Hash(raw))
	return err
}
