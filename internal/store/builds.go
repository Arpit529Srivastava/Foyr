package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// App is a connected repository.
type App struct {
	ID             int64
	InstallationID int64
	GitHubRepoID   int64
	FullName       string
	DefaultBranch  string
	Removed        bool
}

// GetApp returns one app.
func (s *Store) GetApp(ctx context.Context, id int64) (App, error) {
	var a App
	err := s.Pool.QueryRow(ctx, `SELECT id, installation_id, github_repo_id, full_name, default_branch,
		removed_at IS NOT NULL FROM apps WHERE id = $1`, id).
		Scan(&a.ID, &a.InstallationID, &a.GitHubRepoID, &a.FullName, &a.DefaultBranch, &a.Removed)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// SetBuild links a deployment to its CodeBuild run, log and target image, and audits the start.
func (s *Store) SetBuild(ctx context.Context, deploymentID int64, buildID, logURL, image string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		d, err := scanDeployment(tx.QueryRow(ctx, `UPDATE deployments SET build_id = $2, log_url = $3, image_uri = $4
			WHERE id = $1 RETURNING `+deploymentCols, deploymentID, clip(buildID, 200), clip(logURL, 1000),
			clip(image, 500)))
		if err != nil {
			return err
		}
		return audit(ctx, tx, AuditEvent{InstallationID: d.InstallationID, AppID: d.AppID, Actor: System,
			Action: "build.started", TargetType: "deployment", TargetID: strconv.FormatInt(d.ID, 10),
			TraceID: d.TraceID, Details: map[string]any{"build_id": buildID, "image": image}})
	})
}

// Step statuses.
const (
	StepPending   = "pending"
	StepRunning   = "running"
	StepSucceeded = "succeeded"
	StepFailed    = "failed"
	StepSkipped   = "skipped"
)

// Step is one CI-style step of a deployment (ADR-024).
type Step struct {
	Name       string
	Position   int
	Status     string
	Message    string
	LogRef     string
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// Duration is how long the step ran, or zero if it has not finished.
func (s Step) Duration() time.Duration {
	if s.StartedAt == nil || s.FinishedAt == nil {
		return 0
	}
	return s.FinishedAt.Sub(*s.StartedAt)
}

// SyncSteps writes the current state of every step; safe to call on every poll.
func (s *Store) SyncSteps(ctx context.Context, deploymentID int64, steps []Step) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		for _, st := range steps {
			if _, err := tx.Exec(ctx, `INSERT INTO deployment_steps
				(deployment_id, name, position, status, message, log_ref, started_at, finished_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (deployment_id, name) DO UPDATE SET position = EXCLUDED.position,
					status = EXCLUDED.status, message = EXCLUDED.message, log_ref = EXCLUDED.log_ref,
					started_at = EXCLUDED.started_at, finished_at = EXCLUDED.finished_at`,
				deploymentID, st.Name, st.Position, st.Status, clip(st.Message, 2000), clip(st.LogRef, 1000),
				st.StartedAt, st.FinishedAt); err != nil {
				return err
			}
		}
		return nil
	})
}

// Steps returns a deployment's steps in order.
func (s *Store) Steps(ctx context.Context, deploymentID int64) ([]Step, error) {
	rows, err := s.Pool.Query(ctx, `SELECT name, position, status, message, log_ref, started_at, finished_at
		FROM deployment_steps WHERE deployment_id = $1 ORDER BY position`, deploymentID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Step, error) {
		var st Step
		err := r.Scan(&st.Name, &st.Position, &st.Status, &st.Message, &st.LogRef, &st.StartedAt, &st.FinishedAt)
		return st, err
	})
}

// ReleaseJob hands a job back to the queue without counting the attempt, used on shutdown.
func (s *Store) ReleaseJob(ctx context.Context, id int64, worker string) error {
	return s.finishJob(ctx, `UPDATE jobs SET status = 'queued', run_after = now(), locked_by = NULL,
		lease_until = NULL, attempts = greatest(attempts - 1, 0)
		WHERE id = $1 AND locked_by = $2 AND status = 'running'`, id, worker)
}
