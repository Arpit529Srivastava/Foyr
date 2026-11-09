package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Delivery is one webhook GitHub sent, as processed by foyrd.
type Delivery struct {
	ID             string
	Event          string
	Action         string
	InstallationID int64
	RepoFullName   string
	Result         string // ok, ignored or error
	Detail         string
	Duration       time.Duration
	TraceID        string
}

// RecordDelivery stores d; a redelivery of the same ID updates the row and counts the attempt.
func (s *Store) RecordDelivery(ctx context.Context, d Delivery) error {
	if d.ID == "" {
		return nil
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO webhook_deliveries
		(delivery_id, event, action, installation_id, repo_full_name, result, detail, duration_ms, trace_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (delivery_id) DO UPDATE SET
			result = EXCLUDED.result, detail = EXCLUDED.detail, duration_ms = EXCLUDED.duration_ms,
			trace_id = EXCLUDED.trace_id, attempts = webhook_deliveries.attempts + 1, received_at = now()`,
		clip(d.ID, 100), clip(d.Event, 100), clip(d.Action, 100), nullInt(d.InstallationID), clip(d.RepoFullName, 255),
		d.Result, clip(d.Detail, 1000), d.Duration.Milliseconds(), d.TraceID)
	return err
}

// Retention defaults (ADR-026).
const (
	DeliveryRetention = 30 * 24 * time.Hour
	JobRetention      = 7 * 24 * time.Hour
	AuthRetention     = 24 * time.Hour // how long expired tokens, codes and sessions are kept
)

// partitionLock serialises partition creation across foyrd copies.
const partitionLock int64 = 0x666f7973

// EnsureAuditPartitions creates the monthly audit partitions from the month of now through months ahead.
func (s *Store) EnsureAuditPartitions(ctx context.Context, now time.Time, months int) ([]string, error) {
	var created []string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", partitionLock); err != nil {
			return err
		}
		start := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i <= months; i++ {
			from, to := start.AddDate(0, i, 0), start.AddDate(0, i+1, 0)
			name := fmt.Sprintf("audit_events_y%04dm%02d", from.Year(), int(from.Month()))
			var exists bool
			if err := tx.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists); err != nil {
				return err
			}
			if exists {
				continue
			}
			// Names and bounds are generated here, never from input.
			if _, err := tx.Exec(ctx, fmt.Sprintf(
				"CREATE TABLE %s PARTITION OF audit_events FOR VALUES FROM ('%s') TO ('%s')",
				name, from.Format(time.RFC3339), to.Format(time.RFC3339))); err != nil {
				return fmt.Errorf("create %s: %w", name, err)
			}
			created = append(created, name)
		}
		return nil
	})
	return created, err
}

// Pruned counts what Prune removed.
type Pruned struct {
	Deliveries int64
	Jobs       int64
	Auth       int64 // login tokens, auth codes and sessions, for apps and the dashboard
}

// Prune deletes webhook deliveries, finished jobs and expired login rows past retention. Deployments and audit events are kept.
func (s *Store) Prune(ctx context.Context, now time.Time) (Pruned, error) {
	var p Pruned
	tag, err := s.Pool.Exec(ctx, `DELETE FROM webhook_deliveries WHERE received_at < $1`, now.Add(-DeliveryRetention))
	if err != nil {
		return p, err
	}
	p.Deliveries = tag.RowsAffected()
	tag, err = s.Pool.Exec(ctx, `DELETE FROM jobs WHERE status IN ('done', 'failed') AND finished_at < $1`,
		now.Add(-JobRetention))
	if err != nil {
		return p, err
	}
	p.Jobs = tag.RowsAffected()
	cutoff := now.Add(-AuthRetention)
	for _, q := range []string{
		`DELETE FROM login_tokens WHERE expires_at < $1`,
		`DELETE FROM auth_codes WHERE expires_at < $1`,
		`DELETE FROM sessions WHERE expires_at < $1`,
		`DELETE FROM dashboard_login_tokens WHERE expires_at < $1`,
		`DELETE FROM dashboard_sessions WHERE expires_at < $1`,
	} {
		tag, err = s.Pool.Exec(ctx, q, cutoff)
		if err != nil {
			return p, err
		}
		p.Auth += tag.RowsAffected()
	}
	return p, nil
}
