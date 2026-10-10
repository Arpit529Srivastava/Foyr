package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Deployment statuses.
const (
	StatusQueued   = "queued"
	StatusBuilding = "building"
	StatusBuilt    = "built"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"
)

// transitions lists the allowed next statuses for each status.
var transitions = map[string][]string{
	StatusQueued:   {StatusBuilding, StatusFailed, StatusCanceled},
	StatusBuilding: {StatusBuilt, StatusFailed, StatusCanceled},
}

// JobBuild is the job kind that builds a deployment.
const JobBuild = "build"

// NewDeployment is a push that should be built.
type NewDeployment struct {
	InstallationID int64
	AccountLogin   string
	Repo           Repo
	CommitSHA      string
	Branch         string
	CommitMessage  string
	CommitURL      string
	Pusher         Actor
	DeliveryID     string
	TraceID        string
}

// Deployment is one build (and later release) of one commit.
type Deployment struct {
	ID             int64
	AppID          int64
	InstallationID int64
	CommitSHA      string
	Branch         string
	CommitMessage  string
	CommitURL      string
	PusherLogin    string
	Trigger        string
	DeliveryID     string
	TraceID        string
	Status         string
	FailureStep    string
	FailureReason  string
	BuildID        string
	ImageURI       string
	LogURL         string
	CreatedAt      time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
}

const deploymentCols = `id, app_id, installation_id, commit_sha, branch, commit_message, commit_url, pusher_login,
	trigger, delivery_id, trace_id, status, coalesce(failure_step, ''), coalesce(failure_reason, ''),
	build_id, image_uri, log_url, created_at, started_at, finished_at`

func scanDeployment(r pgx.Row) (Deployment, error) {
	var d Deployment
	err := r.Scan(&d.ID, &d.AppID, &d.InstallationID, &d.CommitSHA, &d.Branch, &d.CommitMessage, &d.CommitURL,
		&d.PusherLogin, &d.Trigger, &d.DeliveryID, &d.TraceID, &d.Status, &d.FailureStep, &d.FailureReason,
		&d.BuildID, &d.ImageURI, &d.LogURL, &d.CreatedAt, &d.StartedAt, &d.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// CreateDeployment records the push, its first timeline event, its build job and its audit event
// in one transaction. created is false when the commit already has a live deployment.
func (s *Store) CreateDeployment(ctx context.Context, n NewDeployment) (d Deployment, created bool, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := upsertInstallation(ctx, tx, n.InstallationID, n.AccountLogin); err != nil {
			return err
		}
		appID, appCreated, err := upsertApp(ctx, tx, n.InstallationID, n.Repo)
		if err != nil {
			return err
		}
		if appCreated {
			if err := audit(ctx, tx, AuditEvent{InstallationID: n.InstallationID, AppID: appID, Actor: n.Pusher,
				Action: "app.connected", TargetType: "app", TargetID: strconv.FormatInt(appID, 10),
				TraceID: n.TraceID, Details: map[string]any{"repo": n.Repo.FullName, "via": "push"}}); err != nil {
				return err
			}
		}

		var pusherID *int64
		if id, err := strconv.ParseInt(n.Pusher.ID, 10, 64); err == nil {
			pusherID = &id
		}
		d, err = scanDeployment(tx.QueryRow(ctx, `INSERT INTO deployments
			(app_id, installation_id, commit_sha, branch, commit_message, commit_url, pusher_login, pusher_id,
			 delivery_id, trace_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (app_id, commit_sha) WHERE status IN ('queued', 'building', 'built') DO NOTHING
			RETURNING `+deploymentCols,
			appID, n.InstallationID, n.CommitSHA, clip(n.Branch, 255), clip(n.CommitMessage, 2000),
			clip(n.CommitURL, 500), clip(n.Pusher.Login, 100), pusherID, n.DeliveryID, n.TraceID))
		if errors.Is(err, ErrNotFound) {
			d, err = scanDeployment(tx.QueryRow(ctx, `SELECT `+deploymentCols+` FROM deployments
				WHERE app_id = $1 AND commit_sha = $2 AND status IN ('queued', 'building', 'built')`,
				appID, n.CommitSHA))
			return err
		}
		if err != nil {
			return err
		}
		created = true

		if _, err := tx.Exec(ctx, `INSERT INTO deployment_events (deployment_id, to_status, message)
			VALUES ($1, 'queued', $2)`, d.ID, "Queued from push by "+n.Pusher.Login); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO jobs (kind, deployment_id, installation_id, trace_id)
			VALUES ($1, $2, $3, $4)`, JobBuild, d.ID, n.InstallationID, n.TraceID); err != nil {
			return err
		}
		return audit(ctx, tx, AuditEvent{InstallationID: n.InstallationID, AppID: appID, Actor: n.Pusher,
			Action: "deployment.created", TargetType: "deployment", TargetID: strconv.FormatInt(d.ID, 10),
			TraceID: n.TraceID, Details: map[string]any{"commit": n.CommitSHA, "branch": n.Branch,
				"delivery": n.DeliveryID}})
	})
	return d, created, err
}

// ErrBadTransition is returned when a deployment cannot move to the requested status.
var ErrBadTransition = errors.New("invalid status transition")

// StatusChange moves a deployment to a new status.
type StatusChange struct {
	To      string
	Actor   Actor
	Message string
	Step    string // failed step, for StatusFailed
	Reason  string // failure reason, for StatusFailed
	Image   string // image URI, for StatusBuilt
}

// SetDeploymentStatus applies c, records the timeline event and the audit event in one transaction.
func (s *Store) SetDeploymentStatus(ctx context.Context, id int64, c StatusChange) (Deployment, error) {
	var d Deployment
	err := s.tx(ctx, func(tx pgx.Tx) error {
		cur, err := scanDeployment(tx.QueryRow(ctx, `SELECT `+deploymentCols+` FROM deployments WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if !allowed(cur.Status, c.To) {
			return fmt.Errorf("%w: %s to %s", ErrBadTransition, cur.Status, c.To)
		}
		d, err = scanDeployment(tx.QueryRow(ctx, `UPDATE deployments SET status = $2,
				started_at = CASE WHEN $2 = 'building' THEN now() ELSE started_at END,
				finished_at = CASE WHEN $2 IN ('built', 'failed', 'canceled') THEN now() ELSE finished_at END,
				failure_step = nullif($3, ''), failure_reason = nullif($4, ''),
				image_uri = CASE WHEN $5 = '' THEN image_uri ELSE $5 END
			WHERE id = $1 RETURNING `+deploymentCols, id, c.To, clip(c.Step, 100), clip(c.Reason, 2000), c.Image))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO deployment_events (deployment_id, from_status, to_status, message)
			VALUES ($1, $2, $3, $4)`, id, cur.Status, c.To, clip(c.Message, 2000)); err != nil {
			return err
		}
		details := map[string]any{"from": cur.Status, "to": c.To}
		if c.Step != "" {
			details["step"] = c.Step
		}
		if c.Reason != "" {
			details["reason"] = clip(c.Reason, 500)
		}
		if c.Image != "" {
			details["image"] = c.Image
		}
		return audit(ctx, tx, AuditEvent{InstallationID: cur.InstallationID, AppID: cur.AppID, Actor: c.Actor,
			Action: "deployment." + c.To, TargetType: "deployment", TargetID: strconv.FormatInt(id, 10),
			TraceID: cur.TraceID, Details: details})
	})
	return d, err
}

func allowed(from, to string) bool {
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// GetDeployment returns one deployment.
func (s *Store) GetDeployment(ctx context.Context, id int64) (Deployment, error) {
	return scanDeployment(s.Pool.QueryRow(ctx, `SELECT `+deploymentCols+` FROM deployments WHERE id = $1`, id))
}

// ListDeployments returns an app's deployments newest first; pass the last ID seen as beforeID to page.
func (s *Store) ListDeployments(ctx context.Context, appID, beforeID int64, limit int) ([]Deployment, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+deploymentCols+` FROM deployments
		WHERE app_id = $1 AND ($2 = 0 OR id < $2) ORDER BY id DESC LIMIT $3`, appID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Deployment, error) { return scanDeployment(r) })
}

// DeploymentEvent is one entry in a deployment's timeline.
type DeploymentEvent struct {
	OccurredAt time.Time
	FromStatus string
	ToStatus   string
	Message    string
}

// Timeline returns a deployment's events oldest first.
func (s *Store) Timeline(ctx context.Context, deploymentID int64) ([]DeploymentEvent, error) {
	rows, err := s.Pool.Query(ctx, `SELECT occurred_at, coalesce(from_status, ''), to_status, message
		FROM deployment_events WHERE deployment_id = $1 ORDER BY id`, deploymentID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (DeploymentEvent, error) {
		var e DeploymentEvent
		err := r.Scan(&e.OccurredAt, &e.FromStatus, &e.ToStatus, &e.Message)
		return e, err
	})
}
