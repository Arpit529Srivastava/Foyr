package store

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
)

// Errors for the access rules around members and invites.
var (
	ErrAlreadyMember = errors.New("already a member")
	ErrBadRole       = errors.New("role must be owner, admin, member or viewer")
	ErrBadEmail      = errors.New("not a valid email address")
	ErrBadMode       = errors.New("access mode must be private or public")
	// ErrInvalidToken covers unknown, used, expired and wrong-app tokens, codes and sessions alike.
	ErrInvalidToken = errors.New("invalid or expired token")
)

// Lifetimes of the one-time secrets and the session (ADR-035).
const (
	LoginTokenTTL = 15 * time.Minute
	AuthCodeTTL   = 60 * time.Second
	SessionTTL    = 7 * 24 * time.Hour
)

var roles = []string{"owner", "admin", "member", "viewer"}

// Invite is an email invited to one app. AcceptedAt is nil while it is open.
type Invite struct {
	ID, AppID, InstallationID int64
	Email, Role, InvitedBy    string
	CreatedAt                 time.Time
	AcceptedAt                *time.Time
}

// Member is a user who may open an app.
type Member struct {
	UserID    int64
	Email     string
	Role      string
	CreatedAt time.Time
}

const inviteCols = `id, app_id, installation_id, email, role, invited_by, created_at, accepted_at`

func scanInvite(r pgx.Row) (Invite, error) {
	var i Invite
	err := r.Scan(&i.ID, &i.AppID, &i.InstallationID, &i.Email, &i.Role, &i.InvitedBy, &i.CreatedAt, &i.AcceptedAt)
	return i, err
}

// AppBySlug returns the live app with this slug; removed and unknown apps are ErrNotFound.
func (s *Store) AppBySlug(ctx context.Context, slug string) (App, error) {
	a, err := scanApp(s.Pool.QueryRow(ctx, `SELECT `+appCols+` FROM apps WHERE slug = $1 AND removed_at IS NULL`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// CreateInvite invites email to the app. An open invite for the same email keeps its row and takes the new role.
func (s *Store) CreateInvite(ctx context.Context, appID int64, email, role string, by Actor) (Invite, error) {
	email, ok := authn.NormalizeEmail(email)
	if !ok {
		return Invite{}, ErrBadEmail
	}
	if !slices.Contains(roles, role) {
		return Invite{}, ErrBadRole
	}
	var inv Invite
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var inst int64
		if err := tx.QueryRow(ctx, `SELECT installation_id FROM apps WHERE id = $1 AND removed_at IS NULL`, appID).Scan(&inst); err != nil {
			return notFound(err)
		}
		var member bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_members m JOIN users u ON u.id = m.user_id
			WHERE m.app_id = $1 AND u.email = $2)`, appID, email).Scan(&member); err != nil {
			return err
		}
		if member {
			return ErrAlreadyMember
		}
		var err error
		inv, err = scanInvite(tx.QueryRow(ctx, `INSERT INTO invites (app_id, installation_id, email, role, invited_by)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (app_id, email) WHERE accepted_at IS NULL AND revoked_at IS NULL
			DO UPDATE SET role = EXCLUDED.role, invited_by = EXCLUDED.invited_by
			RETURNING `+inviteCols, appID, inst, email, role, by.Login))
		if err != nil {
			return err
		}
		return audit(ctx, tx, AuditEvent{InstallationID: inst, AppID: appID, Actor: by, Action: "invite.created",
			TargetType: "invite", TargetID: itoa(inv.ID), Details: map[string]any{"email": email, "role": role}})
	})
	return inv, err
}

// RevokeInvite cancels an open invite.
func (s *Store) RevokeInvite(ctx context.Context, inviteID int64, by Actor) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var appID, inst int64
		var email string
		if err := tx.QueryRow(ctx, `UPDATE invites SET revoked_at = now()
			WHERE id = $1 AND accepted_at IS NULL AND revoked_at IS NULL
			RETURNING app_id, installation_id, email`, inviteID).Scan(&appID, &inst, &email); err != nil {
			return notFound(err)
		}
		return audit(ctx, tx, AuditEvent{InstallationID: inst, AppID: appID, Actor: by, Action: "invite.revoked",
			TargetType: "invite", TargetID: itoa(inviteID), Details: map[string]any{"email": email}})
	})
}

// ListMembers returns who may open the app, oldest first.
func (s *Store) ListMembers(ctx context.Context, appID int64) ([]Member, error) {
	rows, err := s.Pool.Query(ctx, `SELECT m.user_id, u.email, m.role, m.created_at
		FROM app_members m JOIN users u ON u.id = m.user_id WHERE m.app_id = $1 ORDER BY m.created_at, m.user_id`, appID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Member, error) {
		var m Member
		err := r.Scan(&m.UserID, &m.Email, &m.Role, &m.CreatedAt)
		return m, err
	})
}

// ListInvites returns the open invites of an app, oldest first.
func (s *Store) ListInvites(ctx context.Context, appID int64) ([]Invite, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+inviteCols+` FROM invites
		WHERE app_id = $1 AND accepted_at IS NULL AND revoked_at IS NULL ORDER BY id`, appID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Invite, error) { return scanInvite(r) })
}

// RemoveMember takes a user off an app and revokes their sessions for it.
func (s *Store) RemoveMember(ctx context.Context, appID, userID int64, by Actor) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var inst int64
		var email string
		if err := tx.QueryRow(ctx, `DELETE FROM app_members m USING users u
			WHERE m.app_id = $1 AND m.user_id = $2 AND u.id = m.user_id
			RETURNING m.installation_id, u.email`, appID, userID).Scan(&inst, &email); err != nil {
			return notFound(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now()
			WHERE app_id = $1 AND user_id = $2 AND revoked_at IS NULL`, appID, userID); err != nil {
			return err
		}
		return audit(ctx, tx, AuditEvent{InstallationID: inst, AppID: appID, Actor: by, Action: "member.removed",
			TargetType: "user", TargetID: itoa(userID), Details: map[string]any{"email": email}})
	})
}

// SetAccessMode makes an app private (members only) or public.
func (s *Store) SetAccessMode(ctx context.Context, appID int64, mode string, by Actor) error {
	if mode != "private" && mode != "public" {
		return ErrBadMode
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		var inst int64
		var old string
		if err := tx.QueryRow(ctx, `UPDATE apps a SET access_mode = $2, updated_at = now()
			FROM (SELECT id, access_mode AS prev FROM apps WHERE id = $1 AND removed_at IS NULL FOR UPDATE) o
			WHERE a.id = o.id RETURNING a.installation_id, o.prev`, appID, mode).Scan(&inst, &old); err != nil {
			return notFound(err)
		}
		return audit(ctx, tx, AuditEvent{InstallationID: inst, AppID: appID, Actor: by, Action: "app.access_changed",
			TargetType: "app", TargetID: itoa(appID), Details: map[string]any{"from": old, "to": mode}})
	})
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// StartLogin stores a login token for email and returns its raw value, but only when the app is public
// or the email is a member or has an open invite. ok is false (and nothing is stored) otherwise.
func (s *Store) StartLogin(ctx context.Context, slug, email string, now time.Time) (raw string, ok bool, err error) {
	email, valid := authn.NormalizeEmail(email)
	if !valid {
		return "", false, nil
	}
	app, err := s.AppBySlug(ctx, slug)
	if errors.Is(err, ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if app.AccessMode != "public" {
		var allowed bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_members m JOIN users u ON u.id = m.user_id
				WHERE m.app_id = $1 AND u.email = $2)
			OR EXISTS (SELECT 1 FROM invites WHERE app_id = $1 AND email = $2 AND accepted_at IS NULL AND revoked_at IS NULL)`,
			app.ID, email).Scan(&allowed); err != nil {
			return "", false, err
		}
		if !allowed {
			return "", false, nil
		}
	}
	raw, hash := authn.NewSecret()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO login_tokens (token_hash, email, app_id, installation_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, hash, email, app.ID, app.InstallationID, now.Add(LoginTokenTTL), now); err != nil {
		return "", false, err
	}
	return raw, true, nil
}

// FinishLogin spends a login token: it creates the user if new, turns an open invite into a membership
// and issues a one-time code for the app's callback. The token works once.
func (s *Store) FinishLogin(ctx context.Context, raw string, now time.Time) (code, slug string, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var email string
		var appID, inst int64
		if err := tx.QueryRow(ctx, `UPDATE login_tokens SET used_at = $2
			WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
			RETURNING email, app_id, installation_id`, authn.Hash(raw), now).Scan(&email, &appID, &inst); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidToken
			}
			return err
		}
		var userID int64
		if err := tx.QueryRow(ctx, `INSERT INTO users (email, last_login_at) VALUES ($1, $2)
			ON CONFLICT (email) DO UPDATE SET last_login_at = EXCLUDED.last_login_at RETURNING id`, email, now).Scan(&userID); err != nil {
			return err
		}
		var role string
		err := tx.QueryRow(ctx, `UPDATE invites SET accepted_at = $3
			WHERE app_id = $1 AND email = $2 AND accepted_at IS NULL AND revoked_at IS NULL RETURNING role`,
			appID, email, now).Scan(&role)
		switch {
		case err == nil:
			if _, err := tx.Exec(ctx, `INSERT INTO app_members (app_id, user_id, installation_id, role) VALUES ($1, $2, $3, $4)
				ON CONFLICT (app_id, user_id) DO NOTHING`, appID, userID, inst, role); err != nil {
				return err
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		var hash []byte
		code, hash = authn.NewSecret()
		if _, err := tx.Exec(ctx, `INSERT INTO auth_codes (code_hash, user_id, app_id, installation_id, expires_at)
			VALUES ($1, $2, $3, $4, $5)`, hash, userID, appID, inst, now.Add(AuthCodeTTL)); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT slug FROM apps WHERE id = $1`, appID).Scan(&slug); err != nil {
			return err
		}
		return audit(ctx, tx, AuditEvent{InstallationID: inst, AppID: appID,
			Actor:  Actor{Type: "foyr_user", ID: itoa(userID), Login: email},
			Action: "user.signed_in", TargetType: "user", TargetID: itoa(userID)})
	})
	if err != nil {
		return "", "", err
	}
	return code, slug, nil
}

// Session is a signed-in user on one app.
type Session struct {
	ID             string
	UserID         int64
	Email, Slug    string
	PublicID       string
	AppID          int64
	InstallationID int64
	ExpiresAt      time.Time
}

// ExchangeCode spends a one-time code for the named app and starts a session.
func (s *Store) ExchangeCode(ctx context.Context, raw, slug string, now time.Time) (Session, error) {
	var sess Session
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `UPDATE auth_codes c SET used_at = $3 FROM apps a, users u
			WHERE c.code_hash = $1 AND c.used_at IS NULL AND c.expires_at > $3
			AND a.id = c.app_id AND a.slug = $2 AND u.id = c.user_id
			RETURNING c.user_id, u.email, a.slug, c.app_id, c.installation_id, u.public_id`,
			authn.Hash(raw), slug, now).Scan(&sess.UserID, &sess.Email, &sess.Slug, &sess.AppID, &sess.InstallationID, &sess.PublicID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidToken
			}
			return err
		}
		sess.ID, _ = authn.NewSecret()
		sess.ExpiresAt = now.Add(SessionTTL)
		_, err := tx.Exec(ctx, `INSERT INTO sessions (id, user_id, app_id, installation_id, created_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6)`, sess.ID, sess.UserID, sess.AppID, sess.InstallationID, now, sess.ExpiresAt)
		return err
	})
	return sess, err
}

// RevokeSession ends one session (sign out). Revoking an unknown or ended session is not an error.
func (s *Store) RevokeSession(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	return err
}

// Access is what authproxy polls: every live app's mode and members, and revoked sessions still unexpired.
func (s *Store) Access(ctx context.Context) (authn.AccessSnapshot, error) {
	snap := authn.AccessSnapshot{Apps: []authn.AppAccess{}, Revoked: []string{}}
	rows, err := s.Pool.Query(ctx, `SELECT id, slug, access_mode FROM apps
		WHERE removed_at IS NULL AND slug IS NOT NULL ORDER BY id`)
	if err != nil {
		return snap, err
	}
	byID := map[int64]int{}
	for rows.Next() {
		var id int64
		a := authn.AppAccess{Members: map[int64]string{}}
		if err := rows.Scan(&id, &a.Slug, &a.Mode); err != nil {
			rows.Close()
			return snap, err
		}
		byID[id] = len(snap.Apps)
		snap.Apps = append(snap.Apps, a)
	}
	if err := rows.Err(); err != nil {
		return snap, err
	}
	members, err := s.Pool.Query(ctx, `SELECT app_id, user_id, role FROM app_members`)
	if err != nil {
		return snap, err
	}
	defer members.Close()
	for members.Next() {
		var appID, userID int64
		var role string
		if err := members.Scan(&appID, &userID, &role); err != nil {
			return snap, err
		}
		if i, ok := byID[appID]; ok {
			snap.Apps[i].Members[userID] = role
		}
	}
	if err := members.Err(); err != nil {
		return snap, err
	}
	revoked, err := s.Pool.Query(ctx, `SELECT id FROM sessions WHERE revoked_at IS NOT NULL AND expires_at > now() ORDER BY id`)
	if err != nil {
		return snap, err
	}
	defer revoked.Close()
	for revoked.Next() {
		var id string
		if err := revoked.Scan(&id); err != nil {
			return snap, err
		}
		snap.Revoked = append(snap.Revoked, id)
	}
	return snap, revoked.Err()
}
