package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
)

// ErrNoSSO is returned when a person may not open an app without signing in to it: they are not an owner or
// admin of its account, or the app or account is unknown, suspended or removed (ADR-039).
var ErrNoSSO = errors.New("no automatic access to this app")

// IssueSSOCode gives a signed-in dashboard person a one-time code for the app's callback, with no email step.
// Only an owner or admin of the account that owns the app gets one. The role is read now, not from the session,
// so removing someone from the account stops it at once. The code is spent by ExchangeCode like any other.
func (s *Store) IssueSSOCode(ctx context.Context, userID int64, slug string, now time.Time) (code string, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var appID, inst int64
		var email string
		if err := tx.QueryRow(ctx, `SELECT a.id, a.installation_id, u.email
			FROM apps a
			JOIN installations i ON i.id = a.installation_id
			JOIN account_members m ON m.installation_id = a.installation_id
			JOIN users u ON u.id = m.user_id
			WHERE a.slug = $1 AND a.removed_at IS NULL
			AND i.removed_at IS NULL AND i.suspended_at IS NULL
			AND m.user_id = $2 AND m.role IN ('owner', 'admin')`, slug, userID).Scan(&appID, &inst, &email); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoSSO
			}
			return err
		}
		var hash []byte
		code, hash = authn.NewSecret()
		if _, err := tx.Exec(ctx, `INSERT INTO auth_codes (code_hash, user_id, app_id, installation_id, expires_at)
			VALUES ($1, $2, $3, $4, $5)`, hash, userID, appID, inst, now.Add(AuthCodeTTL)); err != nil {
			return err
		}
		return audit(ctx, tx, AuditEvent{InstallationID: inst, AppID: appID,
			Actor:  Actor{Type: "foyr_user", ID: itoa(userID), Login: email},
			Action: "user.signed_in", TargetType: "user", TargetID: itoa(userID),
			Details: map[string]any{"method": "sso"}})
	})
	if err != nil {
		return "", err
	}
	return code, nil
}
