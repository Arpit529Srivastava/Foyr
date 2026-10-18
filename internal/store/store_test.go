package store_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/db/dbtest"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

var ctx = context.Background()

func newStore(t *testing.T) *store.Store {
	t.Helper()
	return store.New(dbtest.New(t))
}

func sha(n int) string { return fmt.Sprintf("%040x", n) }

func push(installation, repo int64, commit string) store.NewDeployment {
	return store.NewDeployment{
		InstallationID: installation,
		AccountLogin:   "octo-dev",
		Repo:           store.Repo{ID: repo, FullName: fmt.Sprintf("octo-dev/repo-%d", repo), DefaultBranch: "main", Private: true},
		CommitSHA:      commit,
		Branch:         "main",
		CommitMessage:  "Update page",
		CommitURL:      "https://github.com/octo-dev/repo/commit/" + commit,
		Pusher:         store.GitHubUser(42, "octo-dev"),
		DeliveryID:     "delivery-" + commit[:8],
		TraceID:        "trace-" + commit[:8],
	}
}

func count(t *testing.T, s *store.Store, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := s.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func actions(t *testing.T, s *store.Store, f store.AuditFilter) []string {
	t.Helper()
	evs, err := s.ListAudit(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range evs {
		out = append(out, e.Action)
	}
	return out
}

func TestCreateDeploymentRecordsEverything(t *testing.T) {
	s := newStore(t)
	d, created, err := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if d.Status != store.StatusQueued || d.PusherLogin != "octo-dev" || d.TraceID != "trace-00000000" ||
		d.DeliveryID != "delivery-00000000" || d.CommitMessage != "Update page" {
		t.Fatalf("deployment = %+v", d)
	}
	if n := count(t, s, "SELECT count(*) FROM jobs WHERE deployment_id = $1 AND status = 'queued' AND trace_id = $2",
		d.ID, d.TraceID); n != 1 {
		t.Fatalf("jobs = %d, want 1", n)
	}
	tl, err := s.Timeline(ctx, d.ID)
	if err != nil || len(tl) != 1 || tl[0].ToStatus != store.StatusQueued || !strings.Contains(tl[0].Message, "octo-dev") {
		t.Fatalf("timeline = %+v err=%v", tl, err)
	}
	got := actions(t, s, store.AuditFilter{InstallationID: 1})
	if strings.Join(got, ",") != "deployment.created,app.connected" {
		t.Fatalf("audit = %v", got)
	}
	evs, _ := s.ListAudit(ctx, store.AuditFilter{TargetType: "deployment"})
	if evs[0].Actor.Type != "github_user" || evs[0].Actor.ID != "42" || evs[0].TraceID != d.TraceID ||
		evs[0].Details["commit"] != sha(1) {
		t.Fatalf("audit event = %+v", evs[0])
	}
}

func TestCreateDeploymentIsIdempotent(t *testing.T) {
	s := newStore(t)
	first, _, err := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	if err != nil {
		t.Fatal(err)
	}
	again, created, err := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("redelivery: created=%v id=%d want %d err=%v", created, again.ID, first.ID, err)
	}
	if n := count(t, s, "SELECT count(*) FROM jobs"); n != 1 {
		t.Fatalf("jobs = %d, want 1", n)
	}
	if n := count(t, s, "SELECT count(*) FROM audit_events WHERE action = 'deployment.created'"); n != 1 {
		t.Fatalf("deployment.created events = %d, want 1", n)
	}
}

func TestCreateDeploymentConcurrentRedeliveries(t *testing.T) {
	s := newStore(t)
	if _, _, err := s.CreateDeployment(ctx, push(1, 10, sha(99))); err != nil { // app exists first
		t.Fatal(err)
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		created int
	)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, c, err := s.CreateDeployment(ctx, push(1, 10, sha(1)))
			if err != nil {
				t.Error(err)
			}
			if c {
				mu.Lock()
				created++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if created != 1 {
		t.Fatalf("created %d deployments for one commit, want 1", created)
	}
}

func TestFailedCommitCanBeRebuilt(t *testing.T) {
	s := newStore(t)
	d, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	if _, err := s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusFailed, Actor: store.System}); err != nil {
		t.Fatal(err)
	}
	d2, created, err := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	if err != nil || !created || d2.ID == d.ID {
		t.Fatalf("rebuild after failure: created=%v id=%d err=%v", created, d2.ID, err)
	}
}

func TestCreateDeploymentIsAtomic(t *testing.T) {
	s := newStore(t)
	if _, _, err := s.CreateDeployment(ctx, push(1, 10, "not-a-sha")); err == nil {
		t.Fatal("want error for invalid commit")
	}
	for _, table := range []string{"installations", "apps", "deployments", "jobs", "audit_events"} {
		if n := count(t, s, "SELECT count(*) FROM "+table); n != 0 {
			t.Errorf("%s has %d rows after a failed transaction, want 0", table, n)
		}
	}
}

func TestDeploymentStatusFlow(t *testing.T) {
	s := newStore(t)
	d, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(1)))

	b, err := s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusBuilding, Message: "Build started"})
	if err != nil || b.StartedAt == nil || b.FinishedAt != nil {
		t.Fatalf("building: %+v err=%v", b, err)
	}
	f, err := s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{
		To: store.StatusFailed, Message: "Build failed", Step: "docker build", Reason: "npm ERR! missing script: build"})
	if err != nil || f.FinishedAt == nil || f.FailureStep != "docker build" || !strings.Contains(f.FailureReason, "npm ERR") {
		t.Fatalf("failed: %+v err=%v", f, err)
	}
	if _, err := s.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusBuilt}); !errors.Is(err, store.ErrBadTransition) {
		t.Fatalf("failed -> built: err = %v, want ErrBadTransition", err)
	}

	tl, _ := s.Timeline(ctx, d.ID)
	var steps []string
	for _, e := range tl {
		steps = append(steps, e.FromStatus+">"+e.ToStatus)
	}
	if strings.Join(steps, ",") != ">queued,queued>building,building>failed" {
		t.Fatalf("timeline = %v", steps)
	}
	got := actions(t, s, store.AuditFilter{TargetType: "deployment", TargetID: fmt.Sprint(d.ID)})
	if strings.Join(got, ",") != "deployment.failed,deployment.building,deployment.created" {
		t.Fatalf("audit = %v", got)
	}
	if _, err := s.SetDeploymentStatus(ctx, 999999, store.StatusChange{To: store.StatusBuilding}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing deployment: err = %v", err)
	}
}

func TestAuditEventsAreAppendOnly(t *testing.T) {
	s := newStore(t)
	if _, _, err := s.CreateDeployment(ctx, push(1, 10, sha(1))); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"UPDATE audit_events SET action = 'forged'",
		"DELETE FROM audit_events",
		"TRUNCATE audit_events",
	} {
		_, err := s.Pool.Exec(ctx, sql)
		if err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s: err = %v, want append-only error", sql, err)
		}
	}
}

func TestListAuditPagesWithCursor(t *testing.T) {
	s := newStore(t)
	for i := range 5 {
		if _, _, err := s.CreateDeployment(ctx, push(1, 10, sha(i+1))); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.CreateDeployment(ctx, push(2, 20, sha(100))); err != nil {
		t.Fatal(err)
	}
	page, err := s.ListAudit(ctx, store.AuditFilter{InstallationID: 1, Limit: 4})
	if err != nil || len(page) != 4 {
		t.Fatalf("page 1: %d events err=%v", len(page), err)
	}
	rest, err := s.ListAudit(ctx, store.AuditFilter{InstallationID: 1, Before: page[3].OccurredAt, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range append(page, rest...) {
		if e.InstallationID != 1 {
			t.Fatalf("tenant filter leaked installation %d", e.InstallationID)
		}
	}
	if len(page)+len(rest) > 6 || len(rest) == 0 {
		t.Fatalf("pages = %d + %d, want 6 total for installation 1", len(page), len(rest))
	}
}

func TestSyncInstallationLifecycle(t *testing.T) {
	s := newStore(t)
	user := store.GitHubUser(42, "octo-dev")
	repo := store.Repo{ID: 10, FullName: "octo-dev/foyr-hello", Private: true}

	steps := []store.InstallationChange{
		{Action: "created", InstallationID: 1, AccountLogin: "octo-dev", Actor: user, Added: []store.Repo{repo}},
		{Action: "removed", InstallationID: 1, Actor: user, Removed: []store.Repo{repo}},
		{Action: "added", InstallationID: 1, Actor: user, Added: []store.Repo{repo}},
		{Action: "suspend", InstallationID: 1, Actor: user},
		{Action: "unsuspend", InstallationID: 1, Actor: user},
		{Action: "deleted", InstallationID: 1, AccountLogin: "octo-dev", Actor: user},
	}
	for _, c := range steps {
		if err := s.SyncInstallation(ctx, c); err != nil {
			t.Fatalf("%s: %v", c.Action, err)
		}
	}
	if n := count(t, s, "SELECT count(*) FROM installations WHERE id = 1 AND removed_at IS NOT NULL AND account_login = 'octo-dev'"); n != 1 {
		t.Fatal("installation not marked removed")
	}
	if n := count(t, s, "SELECT count(*) FROM apps WHERE removed_at IS NULL"); n != 0 {
		t.Fatal("apps still active after uninstall")
	}
	got := actions(t, s, store.AuditFilter{InstallationID: 1})
	want := "installation.deleted,installation.unsuspend,installation.suspend,app.reconnected,app.disconnected,app.connected,installation.created"
	if strings.Join(got, ",") != want {
		t.Fatalf("audit = %v\nwant %s", got, want)
	}
}

func TestClaimJobLifecycle(t *testing.T) {
	s := newStore(t)
	if j, err := s.ClaimJob(ctx, "w1", time.Minute, 2); err != nil || j != nil {
		t.Fatalf("empty queue: job=%v err=%v", j, err)
	}
	d, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(1)))

	j, err := s.ClaimJob(ctx, "w1", time.Minute, 2)
	if err != nil || j == nil || j.DeploymentID != d.ID || j.Kind != store.JobBuild || j.Attempts != 1 || j.TraceID != d.TraceID {
		t.Fatalf("claim: %+v err=%v", j, err)
	}
	if other, _ := s.ClaimJob(ctx, "w2", time.Minute, 2); other != nil {
		t.Fatal("a leased job was claimed twice")
	}
	if err := s.CompleteJob(ctx, j.ID, "w2"); !errors.Is(err, store.ErrLeaseLost) {
		t.Fatalf("complete by wrong worker: err = %v", err)
	}
	if err := s.ExtendLease(ctx, j.ID, "w1", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, j.ID, "w1"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM jobs WHERE status = 'done' AND finished_at IS NOT NULL"); n != 1 {
		t.Fatal("job not done")
	}
}

func TestFailJobRetriesThenGivesUp(t *testing.T) {
	s := newStore(t)
	if _, _, err := s.CreateDeployment(ctx, push(1, 10, sha(1))); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("codebuild throttled")
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := s.Pool.Exec(ctx, "UPDATE jobs SET run_after = now()"); err != nil { // skip the backoff wait
			t.Fatal(err)
		}
		j, err := s.ClaimJob(ctx, "w1", time.Minute, 2)
		if err != nil || j == nil || j.Attempts != attempt {
			t.Fatalf("attempt %d: %+v err=%v", attempt, j, err)
		}
		retried, err := s.FailJob(ctx, j.ID, "w1", boom, 30*time.Second)
		if err != nil || retried != (attempt < 3) {
			t.Fatalf("attempt %d: retried=%v err=%v", attempt, retried, err)
		}
		if attempt < 3 {
			if j, _ := s.ClaimJob(ctx, "w1", time.Minute, 2); j != nil {
				t.Fatal("job claimed before its backoff ended")
			}
		}
	}
	if n := count(t, s, "SELECT count(*) FROM jobs WHERE status = 'failed' AND last_error = 'codebuild throttled'"); n != 1 {
		t.Fatal("job should fail for good after max attempts")
	}
}

func TestFailJobWithoutRetryFailsAtOnce(t *testing.T) {
	s := newStore(t)
	_, _, _ = s.CreateDeployment(ctx, push(1, 10, sha(1)))
	j, _ := s.ClaimJob(ctx, "w1", time.Minute, 2)
	retried, err := s.FailJob(ctx, j.ID, "w1", errors.New("customer build failed"), 0)
	if err != nil || retried {
		t.Fatalf("retried=%v err=%v", retried, err)
	}
	if _, err := s.FailJob(ctx, j.ID, "w1", errors.New("again"), 0); !errors.Is(err, store.ErrLeaseLost) {
		t.Fatalf("second fail: err = %v", err)
	}
}

func TestExpiredLeaseIsReclaimed(t *testing.T) {
	s := newStore(t)
	_, _, _ = s.CreateDeployment(ctx, push(1, 10, sha(1)))
	j, _ := s.ClaimJob(ctx, "dead-worker", time.Minute, 2)
	if _, err := s.Pool.Exec(ctx, "UPDATE jobs SET lease_until = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	again, err := s.ClaimJob(ctx, "w2", time.Minute, 2)
	if err != nil || again == nil || again.ID != j.ID || again.Attempts != 2 {
		t.Fatalf("reclaim: %+v err=%v", again, err)
	}
	if err := s.CompleteJob(ctx, j.ID, "dead-worker"); !errors.Is(err, store.ErrLeaseLost) {
		t.Fatalf("dead worker completing: err = %v", err)
	}
}

func TestClaimJobIsFairAcrossTenants(t *testing.T) {
	s := newStore(t)
	for i := range 5 {
		if _, _, err := s.CreateDeployment(ctx, push(1, 10, sha(i+1))); err != nil { // busy customer
			t.Fatal(err)
		}
	}
	if _, _, err := s.CreateDeployment(ctx, push(2, 20, sha(100))); err != nil { // quiet customer, queued last
		t.Fatal(err)
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		claimed = map[int64]int{}
	)
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := s.ClaimJob(ctx, fmt.Sprintf("w%d", i), time.Minute, 2)
			if err != nil {
				t.Error(err)
				return
			}
			if j != nil {
				mu.Lock()
				claimed[j.InstallationID]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	// Some claimers may return early while another holds the tenant lock, so drain once more.
	for range 4 {
		if j, _ := s.ClaimJob(ctx, "drain", time.Minute, 2); j != nil {
			claimed[j.InstallationID]++
		}
	}
	if claimed[1] != 2 || claimed[2] != 1 {
		t.Fatalf("claimed per installation = %v, want 1:2 and 2:1", claimed)
	}
}

func TestBusyTenantDoesNotBlockOthers(t *testing.T) {
	s := newStore(t)
	for i := range 4 {
		if _, _, err := s.CreateDeployment(ctx, push(1, 10, sha(i+1))); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 { // installation 1 is at its limit
		if j, _ := s.ClaimJob(ctx, "w", time.Minute, 2); j == nil || j.InstallationID != 1 {
			t.Fatalf("claim = %+v", j)
		}
	}
	if _, _, err := s.CreateDeployment(ctx, push(2, 20, sha(100))); err != nil {
		t.Fatal(err)
	}
	j, err := s.ClaimJob(ctx, "w", time.Minute, 2)
	if err != nil || j == nil || j.InstallationID != 2 {
		t.Fatalf("next claim = %+v err=%v, want installation 2 to skip past the busy queue", j, err)
	}
}

func TestAuditPartitions(t *testing.T) {
	s := newStore(t)
	now := time.Now().UTC()
	created, err := s.EnsureAuditPartitions(ctx, now, 2)
	if err != nil || len(created) != 3 {
		t.Fatalf("created = %v err=%v", created, err)
	}
	again, err := s.EnsureAuditPartitions(ctx, now, 2)
	if err != nil || len(again) != 0 {
		t.Fatalf("second run created %v err=%v", again, err)
	}
	if _, _, err := s.CreateDeployment(ctx, push(1, 10, sha(1))); err != nil {
		t.Fatal(err)
	}
	var part string
	if err := s.Pool.QueryRow(ctx, "SELECT DISTINCT tableoid::regclass::text FROM audit_events").Scan(&part); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("audit_events_y%04dm%02d", now.Year(), int(now.Month())); part != want {
		t.Fatalf("audit row stored in %s, want %s", part, want)
	}
}

func TestDeliveriesAndPrune(t *testing.T) {
	s := newStore(t)
	d := store.Delivery{ID: "d-1", Event: "push", InstallationID: 1, RepoFullName: "octo-dev/foyr-hello",
		Result: "error", Detail: "database unavailable", Duration: 12 * time.Millisecond, TraceID: "t1"}
	if err := s.RecordDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	d.Result, d.Detail = "ok", "deployment 1 queued"
	if err := s.RecordDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM webhook_deliveries WHERE attempts = 2 AND result = 'ok'"); n != 1 {
		t.Fatal("redelivery not counted")
	}

	dep, _, _ := s.CreateDeployment(ctx, push(1, 10, sha(1)))
	j, _ := s.ClaimJob(ctx, "w1", time.Minute, 2)
	_ = s.CompleteJob(ctx, j.ID, "w1")
	old := time.Now().Add(-40 * 24 * time.Hour)
	_, _ = s.Pool.Exec(ctx, "UPDATE webhook_deliveries SET received_at = $1", old)
	_, _ = s.Pool.Exec(ctx, "UPDATE jobs SET finished_at = $1", old)
	// Recent rows inside retention must survive.
	_ = s.RecordDelivery(ctx, store.Delivery{ID: "d-recent", Event: "push", Result: "ok"})
	_, _, _ = s.CreateDeployment(ctx, push(1, 10, sha(2)))
	j2, _ := s.ClaimJob(ctx, "w1", time.Minute, 2)
	_ = s.CompleteJob(ctx, j2.ID, "w1")

	p, err := s.Prune(ctx, time.Now())
	if err != nil || p.Deliveries != 1 || p.Jobs != 1 {
		t.Fatalf("pruned = %+v err=%v", p, err)
	}
	if n := count(t, s, "SELECT count(*) FROM webhook_deliveries") + count(t, s, "SELECT count(*) FROM jobs"); n != 2 {
		t.Fatalf("recent rows left = %d, want 2", n)
	}
	if _, err := s.GetDeployment(ctx, dep.ID); err != nil {
		t.Fatalf("deployment must survive pruning: %v", err)
	}
}

func TestListDeploymentsPages(t *testing.T) {
	s := newStore(t)
	var appID int64
	for i := range 5 {
		d, _, err := s.CreateDeployment(ctx, push(1, 10, sha(i+1)))
		if err != nil {
			t.Fatal(err)
		}
		appID = d.AppID
	}
	first, err := s.ListDeployments(ctx, appID, 0, 3)
	if err != nil || len(first) != 3 || first[0].CommitSHA != sha(5) {
		t.Fatalf("first page = %d err=%v", len(first), err)
	}
	next, _ := s.ListDeployments(ctx, appID, first[2].ID, 3)
	if len(next) != 2 || next[1].CommitSHA != sha(1) {
		t.Fatalf("second page = %+v", next)
	}
}

func TestListInstallations(t *testing.T) {
	s := newStore(t)
	user := store.GitHubUser(42, "octo-dev")
	for _, c := range []store.InstallationChange{
		{Action: "created", InstallationID: 2, AccountLogin: "zeta-co", Actor: user},
		{Action: "created", InstallationID: 1, AccountLogin: "Acme", Actor: user},
		{Action: "created", InstallationID: 3, AccountLogin: "gone", Actor: user},
		{Action: "deleted", InstallationID: 3, AccountLogin: "gone", Actor: user},
	} {
		if err := s.SyncInstallation(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListInstallations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Removed accounts are left out; the rest sort by login, ignoring case.
	if len(got) != 2 || got[0].Login != "Acme" || got[0].ID != 1 || got[1].Login != "zeta-co" || got[0].CreatedAt.IsZero() {
		t.Fatalf("installations = %+v", got)
	}
}
