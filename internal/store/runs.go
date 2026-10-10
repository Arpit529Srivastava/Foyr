package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// DrainTime is how long a superseded deployment keeps running before it is retired.
const DrainTime = 5 * time.Minute

// SetRun records the task definition and ECS service a deployment runs as.
func (s *Store) SetRun(ctx context.Context, deploymentID int64, taskDefinition, service string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE deployments SET task_definition = $2, ecs_service = $3 WHERE id = $1`,
		deploymentID, clip(taskDefinition, 500), clip(service, 255))
	return err
}

// LiveResult says what MarkLive did.
type LiveResult struct {
	Live       bool  // false when a newer deployment was already live, so this one was superseded at once
	Superseded int64 // the deployment this one replaced, or 0
}

// MarkLive makes a deploying deployment the app's live one in one transaction: the previous live
// deployment becomes superseded and gets a retire job after DrainTime. If a newer deployment went
// live first, this one is superseded instead, so the newest commit always wins.
func (s *Store) MarkLive(ctx context.Context, id int64, actor Actor) (LiveResult, error) {
	var res LiveResult
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var appID, current int64
		if err := tx.QueryRow(ctx, `SELECT a.id, coalesce(a.live_deployment_id, 0) FROM apps a
			JOIN deployments d ON d.app_id = a.id WHERE d.id = $1 FOR UPDATE OF a`, id).Scan(&appID, &current); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if current > id {
			_, d, err := setStatus(ctx, tx, id, StatusChange{To: StatusSuperseded, Actor: actor,
				Message: "Healthy, but a newer deployment is already live"})
			if err != nil {
				return err
			}
			return enqueue(ctx, tx, JobRetire, d, 0)
		}
		if _, _, err := setStatus(ctx, tx, id, StatusChange{To: StatusLive, Actor: actor,
			Message: "Health check passed; now live"}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE apps SET live_deployment_id = $2, updated_at = now() WHERE id = $1`,
			appID, id); err != nil {
			return err
		}
		res.Live = true
		if current == 0 {
			return nil
		}
		_, old, err := setStatus(ctx, tx, current, StatusChange{To: StatusSuperseded, Actor: actor,
			Message: "Replaced by deployment " + strconv.FormatInt(id, 10)})
		if err != nil {
			return err
		}
		res.Superseded = current
		return enqueue(ctx, tx, JobRetire, old, DrainTime)
	})
	return res, err
}

// MarkRetired records that a deployment's service is gone.
func (s *Store) MarkRetired(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		d, err := scanDeployment(tx.QueryRow(ctx, `UPDATE deployments SET retired_at = coalesce(retired_at, now())
			WHERE id = $1 RETURNING `+deploymentCols, id))
		if err != nil {
			return err
		}
		return audit(ctx, tx, AuditEvent{InstallationID: d.InstallationID, AppID: d.AppID, Actor: System,
			Action: "deployment.retired", TargetType: "deployment", TargetID: strconv.FormatInt(id, 10),
			TraceID: d.TraceID, Details: map[string]any{"service": d.ECSService}})
	})
}
