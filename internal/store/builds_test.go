package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

func TestGetApp(t *testing.T) {
	s := newStore(t)
	d, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	a, err := s.GetApp(ctx, d.AppID)
	if err != nil || a.FullName != "octo-dev/repo-10" || a.InstallationID != 1 || a.GitHubRepoID != 10 || a.Removed {
		t.Fatalf("app = %+v err=%v", a, err)
	}
	if _, err := s.GetApp(ctx, 999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing app: err = %v", err)
	}
}

func TestBuildLifecycleIsRecorded(t *testing.T) {
	s := newStore(t)
	d, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	if _, err := s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusBuilding}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBuild(ctx, d.ID, "foyr-build:abc", "https://console.aws/log", "repo/apps/1:d1-0000000"); err != nil {
		t.Fatal(err)
	}
	b, err := s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusBuilt})
	if err != nil {
		t.Fatal(err)
	}
	if b.BuildID != "foyr-build:abc" || b.LogURL != "https://console.aws/log" || b.ImageURI != "repo/apps/1:d1-0000000" {
		t.Fatalf("deployment = %+v", b)
	}
	got := actions(t, s, store.AuditFilter{TargetType: "deployment"})
	want := "deployment.built,build.started,deployment.building,deployment.created"
	if j := join(got); j != want {
		t.Fatalf("audit = %s, want %s", j, want)
	}
}

func TestSyncStepsUpserts(t *testing.T) {
	s := newStore(t)
	d, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	t0 := time.Now().UTC().Truncate(time.Second)
	t1 := t0.Add(90 * time.Second)

	first := []store.Step{
		{Name: "Prepare", Position: 1, Status: store.StepSucceeded, StartedAt: &t0, FinishedAt: &t0},
		{Name: "Build image", Position: 2, Status: store.StepRunning, StartedAt: &t0},
	}
	if err := s.SyncSteps(ctx, d.ID, first); err != nil {
		t.Fatal(err)
	}
	second := []store.Step{
		{Name: "Prepare", Position: 1, Status: store.StepSucceeded, StartedAt: &t0, FinishedAt: &t0},
		{Name: "Build image", Position: 2, Status: store.StepFailed, Message: "exit status 1", LogRef: "link",
			StartedAt: &t0, FinishedAt: &t1},
	}
	if err := s.SyncSteps(ctx, d.ID, second); err != nil {
		t.Fatal(err)
	}
	steps, err := s.Steps(ctx, d.ID)
	if err != nil || len(steps) != 2 {
		t.Fatalf("steps = %+v err=%v", steps, err)
	}
	if b := steps[1]; b.Status != store.StepFailed || b.Message != "exit status 1" || b.Duration() != 90*time.Second {
		t.Fatalf("build step = %+v (duration %v)", b, b.Duration())
	}
	if steps[0].Name != "Prepare" {
		t.Fatalf("order = %v", steps)
	}
}

func TestReleaseJobDoesNotCountAttempt(t *testing.T) {
	s := newStore(t)
	_, _, _ = s.CreateDeployment(ctx, push(1, 10, sha(1)))
	j, _ := s.ClaimJob(ctx, "w1", time.Minute, 2)
	if err := s.ReleaseJob(ctx, j.ID, "w1"); err != nil {
		t.Fatal(err)
	}
	again, err := s.ClaimJob(ctx, "w2", time.Minute, 2)
	if err != nil || again == nil || again.ID != j.ID || again.Attempts != 1 {
		t.Fatalf("after release: %+v err=%v", again, err)
	}
	if err := s.ReleaseJob(ctx, j.ID, "w1"); !errors.Is(err, store.ErrLeaseLost) {
		t.Fatalf("release by old worker: err = %v", err)
	}
}

func join(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

func TestSetLogURL(t *testing.T) {
	s := newStore(t)
	d, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	if err := s.SetLogURL(ctx, d.ID, "https://logs/exact"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetDeployment(ctx, d.ID); got.LogURL != "https://logs/exact" {
		t.Fatalf("log url = %q", got.LogURL)
	}
}

func TestListAppsSkipsRemovedAndSortsByName(t *testing.T) {
	s := newStore(t)
	s.CreateDeployment(ctx, push(1, 20, sha(1)))
	s.CreateDeployment(ctx, push(1, 10, sha(2)))
	s.CreateDeployment(ctx, push(2, 30, sha(3)))
	if err := s.SyncInstallation(ctx, store.InstallationChange{InstallationID: 2, AccountLogin: "octo-dev", Action: "deleted"}); err != nil {
		t.Fatal(err)
	}
	apps, err := s.ListApps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range apps {
		names = append(names, a.FullName)
	}
	if j := join(names); j != "octo-dev/repo-10,octo-dev/repo-20" {
		t.Fatalf("apps = %s", j)
	}
}

func TestRecentDeploymentsAcrossApps(t *testing.T) {
	s := newStore(t)
	a, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	b, _, _ := s.CreateDeployment(ctx, push(2, 20, sha(2)))
	c, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(3)))
	got, err := s.RecentDeployments(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != c.ID || got[1].ID != b.ID || got[0].Slug == "" || got[1].AppName != "octo-dev/repo-20" {
		t.Fatalf("recent = %+v (a=%d)", got, a.ID)
	}
}
