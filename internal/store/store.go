// Package store reads and writes foyrd's platform data in Postgres.
// Every state change writes its audit event in the same transaction (ADR-026).
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the Postgres-backed data layer.
type Store struct {
	Pool *pgxpool.Pool
}

// New returns a Store on pool.
func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

// Actor is who caused a change.
type Actor struct {
	Type  string // github_user, foyr_user or system
	ID    string
	Login string
}

// System is the actor for changes foyrd makes on its own.
var System = Actor{Type: "system", Login: "foyr"}

// GitHubUser returns the actor for a GitHub account.
func GitHubUser(id int64, login string) Actor {
	if id == 0 && login == "" {
		return System
	}
	return Actor{Type: "github_user", ID: strconv.FormatInt(id, 10), Login: login}
}

// AuditEvent is one row of the audit trail.
type AuditEvent struct {
	OccurredAt     time.Time
	InstallationID int64
	AppID          int64
	Actor          Actor
	Action         string
	TargetType     string
	TargetID       string
	TraceID        string
	Details        map[string]any
}

// querier is satisfied by both the pool and a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// audit appends e inside q's transaction.
func audit(ctx context.Context, q querier, e AuditEvent) error {
	details := e.Details
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("audit details: %w", err)
	}
	actor := e.Actor
	if actor.Type == "" {
		actor = System
	}
	_, err = q.Exec(ctx, `INSERT INTO audit_events
		(installation_id, app_id, actor_type, actor_id, actor_login, action, target_type, target_id, trace_id, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		nullInt(e.InstallationID), nullInt(e.AppID), actor.Type, actor.ID, actor.Login,
		e.Action, e.TargetType, e.TargetID, e.TraceID, raw)
	if err != nil {
		return fmt.Errorf("audit %s: %w", e.Action, err)
	}
	return nil
}

// AuditFilter narrows ListAudit; zero fields match everything.
type AuditFilter struct {
	InstallationID int64
	TargetType     string
	TargetID       string
	Before         time.Time // cursor: only events strictly older
	Limit          int
}

// ListAudit returns audit events newest first, using the (tenant, time) indexes.
func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]AuditEvent, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	var (
		where []string
		args  []any
	)
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.InstallationID != 0 {
		add("installation_id = $%d", f.InstallationID)
	}
	if f.TargetType != "" {
		add("target_type = $%d", f.TargetType)
	}
	if f.TargetID != "" {
		add("target_id = $%d", f.TargetID)
	}
	if !f.Before.IsZero() {
		add("occurred_at < $%d", f.Before)
	}
	sql := `SELECT occurred_at, coalesce(installation_id, 0), coalesce(app_id, 0), actor_type, actor_id, actor_login,
		action, target_type, target_id, trace_id, details FROM audit_events`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, f.Limit)
	sql += fmt.Sprintf(" ORDER BY occurred_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (AuditEvent, error) {
		var e AuditEvent
		err := r.Scan(&e.OccurredAt, &e.InstallationID, &e.AppID, &e.Actor.Type, &e.Actor.ID, &e.Actor.Login,
			&e.Action, &e.TargetType, &e.TargetID, &e.TraceID, &e.Details)
		return e, err
	})
}

// tx runs fn in a transaction.
func (s *Store) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.Pool, fn)
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// clip shortens s to at most n bytes without splitting a character.
func clip(s string, n int) string {
	s = strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "")
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")
