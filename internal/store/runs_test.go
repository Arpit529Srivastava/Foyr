package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// deploying creates a deployment for commit n and moves it to deploying.
func deploying(t *testing.T, s *store.Store, n int) store.Deployment {
	t.Helper()
	d, _, err := s.CreateDeployment(ctx, push(1, 10, sha(n)))
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{store.StatusBuilding, store.StatusBuilt, store.StatusDeploying} {
		if d, err = s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: to}); err != nil {
			t.Fatalf("%s: %v", to, err)
		}
	}
	return d
}

func liveID(t *testing.T, s *store.Store, appID int64) int64 {
	t.Helper()
	a, err := s.GetApp(ctx, appID)
	if err != nil {
		t.Fatal(err)
	}
	return a.LiveDeployment
}

func TestBuiltQueuesDeployJob(t *testing.T) {
	s := newStore(t)
	d, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	_, _ = s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusBuilding})
	if _, err := s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusBuilt}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM jobs WHERE kind = 'deploy' AND deployment_id = $1 AND trace_id = $2",
		d.ID, d.TraceID); n != 1 {
		t.Fatalf("deploy jobs = %d, want 1", n)
	}
}

func TestMarkLiveSupersedesPrevious(t *testing.T) {
	s := newStore(t)
	d1 := deploying(t, s, 1)
	res, err := s.MarkLive(ctx, d1.ID, store.System)
	if err != nil || !res.Live || res.Superseded != 0 || liveID(t, s, d1.AppID) != d1.ID {
		t.Fatalf("first live: %+v err=%v", res, err)
	}
	if got, _ := s.GetDeployment(ctx, d1.ID); got.Status != store.StatusLive || got.LiveAt == nil {
		t.Fatalf("d1 = %+v", got)
	}

	d2 := deploying(t, s, 2)
	res, err = s.MarkLive(ctx, d2.ID, store.System)
	if err != nil || !res.Live || res.Superseded != d1.ID || liveID(t, s, d1.AppID) != d2.ID {
		t.Fatalf("second live: %+v err=%v", res, err)
	}
	if got, _ := s.GetDeployment(ctx, d1.ID); got.Status != store.StatusSuperseded {
		t.Fatalf("d1 status = %s", got.Status)
	}
	var wait time.Duration
	if err := s.Pool.QueryRow(ctx, `SELECT run_after - now() FROM jobs WHERE kind = 'retire' AND deployment_id = $1`,
		d1.ID).Scan(&wait); err != nil || wait < 4*time.Minute {
		t.Fatalf("retire job for d1 runs in %v (err %v), want about %v", wait, err, store.DrainTime)
	}
	got := actions(t, s, store.AuditFilter{TargetType: "deployment", TargetID: "1"})
	if got[0] != "deployment.superseded" || got[1] != "deployment.live" {
		t.Fatalf("d1 audit = %v", got)
	}
}

func TestLateFinisherIsSupersededAtOnce(t *testing.T) {
	s := newStore(t)
	older, newer := deploying(t, s, 1), deploying(t, s, 2)
	if _, err := s.MarkLive(ctx, newer.ID, store.System); err != nil {
		t.Fatal(err)
	}
	res, err := s.MarkLive(ctx, older.ID, store.System)
	if err != nil || res.Live {
		t.Fatalf("older going live late: %+v err=%v", res, err)
	}
	if liveID(t, s, older.AppID) != newer.ID {
		t.Fatal("an older deployment replaced a newer live one")
	}
	if got, _ := s.GetDeployment(ctx, older.ID); got.Status != store.StatusSuperseded {
		t.Fatalf("older status = %s", got.Status)
	}
	if n := count(t, s, "SELECT count(*) FROM jobs WHERE kind = 'retire' AND deployment_id = $1 AND run_after <= now()",
		older.ID); n != 1 {
		t.Fatal("late finisher should be retired straight away")
	}
}

func TestRedeliveryOfLiveCommitDoesNotRebuild(t *testing.T) {
	s := newStore(t)
	d := deploying(t, s, 1)
	_, _ = s.MarkLive(ctx, d.ID, store.System)
	again, created, err := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	if err != nil || created || again.ID != d.ID {
		t.Fatalf("created=%v id=%d err=%v", created, again.ID, err)
	}
}

func TestRunRecordsAndRetire(t *testing.T) {
	s := newStore(t)
	d := deploying(t, s, 1)
	if err := s.SetRun(ctx, d.ID, "arn:task-def/app-1:3", "d-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRetired(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetDeployment(ctx, d.ID)
	if got.TaskDefinition != "arn:task-def/app-1:3" || got.ECSService != "d-1" {
		t.Fatalf("deployment = %+v", got)
	}
	if n := count(t, s, "SELECT count(*) FROM deployments WHERE id = $1 AND retired_at IS NOT NULL", d.ID); n != 1 {
		t.Fatal("retired_at not set")
	}
	if a := actions(t, s, store.AuditFilter{TargetType: "deployment"}); a[0] != "deployment.retired" {
		t.Fatalf("audit = %v", a)
	}
}

func TestLiveCannotGoBack(t *testing.T) {
	s := newStore(t)
	d := deploying(t, s, 1)
	_, _ = s.MarkLive(ctx, d.ID, store.System)
	if _, err := s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusDeploying}); !errors.Is(err, store.ErrBadTransition) {
		t.Fatalf("live -> deploying: err = %v", err)
	}
}

func TestFailedDeployKeepsReason(t *testing.T) {
	s := newStore(t)
	d := deploying(t, s, 1)
	f, err := s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusFailed, Step: "Health check",
		Reason: "Essential container in task exited"})
	if err != nil || f.FailureStep != "Health check" || f.FailureReason != "Essential container in task exited" {
		t.Fatalf("failed = %+v err=%v", f, err)
	}
	if liveID(t, s, d.AppID) != 0 {
		t.Fatal("a failed deployment must not become live")
	}
}
