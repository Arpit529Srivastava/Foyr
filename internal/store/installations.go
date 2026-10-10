package store

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/Arpit529Srivastava/Foyr/internal/slug"
)

// Repo is a GitHub repository as webhooks describe it.
type Repo struct {
	ID            int64
	FullName      string
	DefaultBranch string // empty in installation events
	Private       bool
}

// InstallationChange is an installation or installation_repositories webhook.
type InstallationChange struct {
	Action         string
	InstallationID int64
	AccountLogin   string
	Actor          Actor
	TraceID        string
	Added          []Repo
	Removed        []Repo
}

// SyncInstallation records an installation change, its apps and their audit events.
func (s *Store) SyncInstallation(ctx context.Context, c InstallationChange) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		ev := func(action, targetType, targetID string, appID int64, details map[string]any) error {
			return audit(ctx, tx, AuditEvent{InstallationID: c.InstallationID, AppID: appID, Actor: c.Actor,
				Action: action, TargetType: targetType, TargetID: targetID, TraceID: c.TraceID, Details: details})
		}
		instID := strconv.FormatInt(c.InstallationID, 10)

		switch c.Action {
		case "deleted":
			if _, err := tx.Exec(ctx, `UPDATE installations SET removed_at = now(), updated_at = now() WHERE id = $1`,
				c.InstallationID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE apps SET removed_at = now(), updated_at = now()
				WHERE installation_id = $1 AND removed_at IS NULL`, c.InstallationID); err != nil {
				return err
			}
			return ev("installation.deleted", "installation", instID, 0, map[string]any{"account": c.AccountLogin})
		case "suspend", "unsuspend":
			if err := upsertInstallation(ctx, tx, c.InstallationID, c.AccountLogin); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE installations
				SET suspended_at = CASE WHEN $2 THEN now() END, updated_at = now() WHERE id = $1`,
				c.InstallationID, c.Action == "suspend"); err != nil {
				return err
			}
			return ev("installation."+c.Action, "installation", instID, 0, nil)
		}

		if err := upsertInstallation(ctx, tx, c.InstallationID, c.AccountLogin); err != nil {
			return err
		}
		if c.Action == "created" {
			if err := ev("installation.created", "installation", instID, 0,
				map[string]any{"account": c.AccountLogin}); err != nil {
				return err
			}
		} else if c.Action != "added" && c.Action != "removed" {
			if err := ev("installation."+c.Action, "installation", instID, 0, nil); err != nil {
				return err
			}
		}
		for _, r := range c.Added {
			id, created, err := upsertApp(ctx, tx, c.InstallationID, r)
			if err != nil {
				return err
			}
			action := "app.connected"
			if !created {
				action = "app.reconnected"
			}
			if err := ev(action, "app", strconv.FormatInt(id, 10), id, map[string]any{"repo": r.FullName}); err != nil {
				return err
			}
		}
		for _, r := range c.Removed {
			var id int64
			err := tx.QueryRow(ctx, `UPDATE apps SET removed_at = now(), updated_at = now()
				WHERE github_repo_id = $1 RETURNING id`, r.ID).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if err := ev("app.disconnected", "app", strconv.FormatInt(id, 10), id,
				map[string]any{"repo": r.FullName}); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertInstallation(ctx context.Context, q querier, id int64, login string) error {
	_, err := q.Exec(ctx, `INSERT INTO installations (id, account_login) VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET
			account_login = CASE WHEN EXCLUDED.account_login = '' THEN installations.account_login
			                     ELSE EXCLUDED.account_login END,
			removed_at = NULL, updated_at = now()`, id, login)
	return err
}

// upsertApp creates or revives the app for r and reports whether it is new.
func upsertApp(ctx context.Context, q querier, installationID int64, r Repo) (id int64, created bool, err error) {
	err = q.QueryRow(ctx, `INSERT INTO apps (installation_id, github_repo_id, full_name, default_branch, private)
		VALUES ($1, $2, $3, coalesce(nullif($4, ''), 'main'), $5)
		ON CONFLICT (github_repo_id) DO UPDATE SET
			installation_id = EXCLUDED.installation_id,
			full_name = EXCLUDED.full_name,
			default_branch = CASE WHEN $4 = '' THEN apps.default_branch ELSE EXCLUDED.default_branch END,
			private = EXCLUDED.private,
			removed_at = NULL, updated_at = now()
		RETURNING id, xmax = 0`, installationID, r.ID, r.FullName, r.DefaultBranch, r.Private).Scan(&id, &created)
	if err != nil {
		return id, created, err
	}
	return id, created, assignSlug(ctx, q, id, r.FullName)
}

// assignSlug gives an app without a slug the first free candidate, or app-<id> if all are taken.
func assignSlug(ctx context.Context, q querier, appID int64, fullName string) error {
	candidates := append(slug.Candidates(fullName), "app-"+strconv.FormatInt(appID, 10))
	_, err := q.Exec(ctx, `UPDATE apps SET slug = (
			SELECT c FROM unnest($2::text[]) WITH ORDINALITY AS t(c, n)
			WHERE NOT EXISTS (SELECT 1 FROM apps WHERE slug = c) ORDER BY n LIMIT 1)
		WHERE id = $1 AND slug IS NULL`, appID, candidates)
	return err
}
