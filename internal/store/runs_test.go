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

func TestAppsGetUniqueSlugs(t *testing.T) {
	s := newStore(t)
	mk := func(installation, repo int64, name string) store.App {
		t.Helper()
		n := push(installation, repo, sha(int(repo)))
		n.Repo.FullName = name
		d, _, err := s.CreateDeployment(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := s.GetApp(ctx, d.AppID)
		return a
	}
	first := mk(1, 10, "octo-dev/foyr-hello")
	second := mk(2, 20, "someone-else/Foyr_Hello") // same name, other owner
	reserved := mk(1, 30, "octo-dev/admin")
	if first.Slug != "foyr-hello" || second.Slug != "foyr-hello-2" || reserved.Slug != "admin-app" {
		t.Fatalf("slugs = %q, %q, %q", first.Slug, second.Slug, reserved.Slug)
	}
	// Reconnecting keeps the slug.
	if err := s.SyncInstallation(ctx, store.InstallationChange{Action: "added", InstallationID: 1,
		Added: []store.Repo{{ID: 10, FullName: "octo-dev/foyr-hello"}}}); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.GetApp(ctx, first.ID); a.Slug != "foyr-hello" {
		t.Fatalf("slug after reconnect = %q", a.Slug)
	}
}

func TestRoutesListOnlyLiveApps(t *testing.T) {
	s := newStore(t)
	live := deploying(t, s, 1)
	_ = s.SetRun(ctx, live.ID, "arn:td", "d-1")
	if _, err := s.MarkLive(ctx, live.ID, store.System); err != nil {
		t.Fatal(err)
	}
	other, _, _ := s.CreateDeployment(ctx, push(2, 20, sha(50))) // queued, never live

	routes, err := s.Routes(ctx)
	if err != nil || len(routes) != 1 {
		t.Fatalf("routes = %+v err=%v", routes, err)
	}
	if r := routes[0]; r.Slug != "repo-10" || r.Service != "d-1" || r.DeploymentID != live.ID || r.AppID != live.AppID {
		t.Fatalf("route = %+v", r)
	}
	_ = other

	// Uninstalling hides the app.
	_ = s.SyncInstallation(ctx, store.InstallationChange{Action: "deleted", InstallationID: 1})
	if routes, _ := s.Routes(ctx); len(routes) != 0 {
		t.Fatalf("removed app still routed: %+v", routes)
	}
}
