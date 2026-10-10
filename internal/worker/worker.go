// Package worker runs queued jobs: claim, keep the lease alive, then complete, retry or release.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// Queue is the job queue; *store.Store implements it, and SQS could later (ADR-025).
type Queue interface {
	ClaimJob(ctx context.Context, worker string, lease time.Duration, maxPerTenant int) (*store.Job, error)
	ExtendLease(ctx context.Context, id int64, worker string, lease time.Duration) error
	CompleteJob(ctx context.Context, id int64, worker string) error
	FailJob(ctx context.Context, id int64, worker string, cause error, retryAfter time.Duration) (bool, error)
	ReleaseJob(ctx context.Context, id int64, worker string) error
}

// Runner does the work of one job.
type Runner interface {
	Run(ctx context.Context, job store.Job) error
	GiveUp(ctx context.Context, job store.Job, cause error)
}

// Pool runs Workers goroutines that share the queue.
type Pool struct {
	Queue        Queue
	Runner       Runner
	Log          *slog.Logger
	Name         string        // prefix for worker names, e.g. the host
	Workers      int           // default 2
	Lease        time.Duration // default 2m
	Heartbeat    time.Duration // default Lease/4
	Idle         time.Duration // wait when the queue is empty; default 2s
	MaxPerTenant int           // default store.DefaultMaxRunningPerTenant
}

// Backoff is the wait before retry n (1-based) of a failed job.
func Backoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 30 * time.Second
	case 2:
		return 2 * time.Minute
	default:
		return 5 * time.Minute
	}
}

// Run blocks until ctx ends and every worker has handed back its job.
func (p *Pool) Run(ctx context.Context) {
	p.defaults()
	var wg sync.WaitGroup
	for i := range p.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.loop(ctx, fmt.Sprintf("%s-%d", p.Name, i+1))
		}()
	}
	wg.Wait()
}

func (p *Pool) defaults() {
	if p.Workers <= 0 {
		p.Workers = 2
	}
	if p.Lease <= 0 {
		p.Lease = 2 * time.Minute
	}
	if p.Heartbeat <= 0 {
		p.Heartbeat = p.Lease / 4
	}
	if p.Idle <= 0 {
		p.Idle = 2 * time.Second
	}
	if p.MaxPerTenant <= 0 {
		p.MaxPerTenant = store.DefaultMaxRunningPerTenant
	}
	if p.Log == nil {
		p.Log = slog.Default()
	}
}

func (p *Pool) loop(ctx context.Context, name string) {
	log := p.Log.With("worker", name)
	for ctx.Err() == nil {
		job, err := p.Queue.ClaimJob(ctx, name, p.Lease, p.MaxPerTenant)
		if err != nil && ctx.Err() == nil {
			log.Error("claim failed", "err", err)
		}
		if job == nil {
			sleep(ctx, p.Idle)
			continue
		}
		p.handle(ctx, log, name, *job)
	}
}

// handle runs one job while a heartbeat keeps its lease, then records the outcome.
func (p *Pool) handle(ctx context.Context, log *slog.Logger, name string, job store.Job) {
	log = log.With("job_id", job.ID, "kind", job.Kind, "deploy_id", job.DeploymentID,
		"trace_id", job.TraceID, "attempt", job.Attempts)
	runCtx, cancel := context.WithCancelCause(obs.WithLogger(ctx, log))
	defer cancel(nil)
	go p.heartbeat(runCtx, cancel, log, name, job.ID)

	log.Info("job started")
	start := time.Now()
	err := p.run(runCtx, job)
	done := context.WithoutCancel(runCtx)
	took := slog.Int64("duration_ms", time.Since(start).Milliseconds())

	switch {
	case err == nil:
		if err := p.Queue.CompleteJob(done, job.ID, name); err != nil {
			log.Error("complete failed", "err", err)
		}
		log.Info("job done", took)
	case errors.Is(context.Cause(runCtx), store.ErrLeaseLost):
		log.Warn("job lease lost; another worker owns it now", took)
	case ctx.Err() != nil:
		if err := p.Queue.ReleaseJob(done, job.ID, name); err != nil {
			log.Warn("release on shutdown failed", "err", err)
		}
		log.Info("job released for shutdown", took)
	default:
		retry := Backoff(job.Attempts)
		if errors.Is(err, errUnknownKind) {
			retry = 0
		}
		retried, ferr := p.Queue.FailJob(done, job.ID, name, err, retry)
		if ferr != nil {
			log.Error("record failure failed", "err", ferr, "cause", err)
			return
		}
		if retried {
			log.Warn("job failed; will retry", "err", err, "retry_in", retry.String(), took)
			return
		}
		log.Error("job failed; giving up", "err", err, took)
		p.Runner.GiveUp(done, job, err)
	}
}

var errUnknownKind = errors.New("unknown job kind")

// run dispatches by kind.
func (p *Pool) run(ctx context.Context, job store.Job) error {
	if job.Kind != store.JobBuild {
		return fmt.Errorf("%w %q", errUnknownKind, job.Kind)
	}
	return p.Runner.Run(ctx, job)
}

// heartbeat extends the lease until ctx ends, and cancels the job if the lease is lost.
func (p *Pool) heartbeat(ctx context.Context, cancel context.CancelCauseFunc, log *slog.Logger, name string, id int64) {
	t := time.NewTicker(p.Heartbeat)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := p.Queue.ExtendLease(ctx, id, name, p.Lease)
			if errors.Is(err, store.ErrLeaseLost) {
				cancel(store.ErrLeaseLost)
				return
			}
			if err != nil && ctx.Err() == nil {
				log.Warn("extend lease failed", "err", err)
			}
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
