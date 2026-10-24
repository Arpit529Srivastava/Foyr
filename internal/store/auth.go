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
