package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Job is a unit of background work leased to one worker at a time.
type Job struct {
	ID             int64
	Kind           string
	DeploymentID   int64
	InstallationID int64
	TraceID        string
	Attempts       int
	MaxAttempts    int
	LeaseUntil     time.Time
}

// ErrLeaseLost is returned when a worker touches a job it no longer holds.
var ErrLeaseLost = errors.New("job lease lost")

// DefaultMaxRunningPerTenant caps how many jobs one installation runs at once (ADR-026).
const DefaultMaxRunningPerTenant = 2

// ClaimJob leases the next ready job to worker for lease, or returns nil when none is ready.
// A job is ready when queued and due, or running with an expired lease (its worker died).
// No installation runs more than maxPerTenant jobs at once, so one busy customer cannot starve the rest.
func (s *Store) ClaimJob(ctx context.Context, worker string, lease time.Duration, maxPerTenant int) (*Job, error) {
	if maxPerTenant <= 0 {
		maxPerTenant = DefaultMaxRunningPerTenant
	}
	var job *Job
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var id, installation int64
		err := tx.QueryRow(ctx, `SELECT j.id, j.installation_id FROM jobs j
			WHERE ((j.status = 'queued' AND j.run_after <= now())
			    OR (j.status = 'running' AND j.lease_until < now()))
			  AND (SELECT count(*) FROM jobs r WHERE r.installation_id = j.installation_id
			       AND r.status = 'running' AND r.lease_until >= now()) < $1
			ORDER BY j.run_after, j.id
			FOR UPDATE SKIP LOCKED LIMIT 1`, maxPerTenant).Scan(&id, &installation)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		// Serialise claims per tenant, then recount, so two workers cannot both take the last slot.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('claim:' || $1::bigint, 0))`,
			installation); err != nil {
			return err
		}
		var running int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE installation_id = $1
			AND status = 'running' AND lease_until >= now() AND id <> $2`, installation, id).Scan(&running); err != nil {
			return err
		}
		if running >= maxPerTenant {
			return nil
		}
		j := &Job{}
		if err := tx.QueryRow(ctx, `UPDATE jobs SET status = 'running', locked_by = $2,
				lease_until = now() + make_interval(secs => $3), attempts = attempts + 1,
				started_at = coalesce(started_at, now())
			WHERE id = $1
			RETURNING id, kind, coalesce(deployment_id, 0), installation_id, trace_id, attempts, max_attempts, lease_until`,
			id, worker, lease.Seconds()).Scan(&j.ID, &j.Kind, &j.DeploymentID, &j.InstallationID, &j.TraceID,
			&j.Attempts, &j.MaxAttempts, &j.LeaseUntil); err != nil {
			return err
		}
		job = j
		return nil
	})
	return job, err
}

// ExtendLease keeps a long job leased to worker.
func (s *Store) ExtendLease(ctx context.Context, id int64, worker string, lease time.Duration) error {
	return s.finishJob(ctx, `UPDATE jobs SET lease_until = now() + make_interval(secs => $3)
		WHERE id = $1 AND locked_by = $2 AND status = 'running'`, id, worker, lease.Seconds())
}

// CompleteJob marks a job done.
func (s *Store) CompleteJob(ctx context.Context, id int64, worker string) error {
	return s.finishJob(ctx, `UPDATE jobs SET status = 'done', finished_at = now(), lease_until = NULL, last_error = NULL
		WHERE id = $1 AND locked_by = $2 AND status = 'running'`, id, worker)
}

// FailJob records cause and requeues the job after retryAfter, or fails it for good when
// retryAfter is zero or attempts are used up. retried reports which happened.
func (s *Store) FailJob(ctx context.Context, id int64, worker string, cause error, retryAfter time.Duration) (retried bool, err error) {
	msg := clip(cause.Error(), 2000)
	err = s.Pool.QueryRow(ctx, `UPDATE jobs SET
			status = CASE WHEN $4 > 0 AND attempts < max_attempts THEN 'queued' ELSE 'failed' END,
			run_after = now() + make_interval(secs => $4),
			finished_at = CASE WHEN $4 > 0 AND attempts < max_attempts THEN NULL ELSE now() END,
			lease_until = NULL, locked_by = NULL, last_error = $3
		WHERE id = $1 AND locked_by = $2 AND status = 'running'
		RETURNING status = 'queued'`, id, worker, msg, retryAfter.Seconds()).Scan(&retried)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrLeaseLost
	}
	return retried, err
}

func (s *Store) finishJob(ctx context.Context, sql string, args ...any) error {
	tag, err := s.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	return nil
}
