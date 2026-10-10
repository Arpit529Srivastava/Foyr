package worker

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/db/dbtest"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

type fakeRunner struct {
	runs    atomic.Int32
	run     func(ctx context.Context, job store.Job) error
	mu      sync.Mutex
	gaveUp  []error
	running chan struct{}
}

func (f *fakeRunner) Run(ctx context.Context, job store.Job) error {
	f.runs.Add(1)
	if f.running != nil {
		select {
		case f.running <- struct{}{}:
		default:
		}
	}
	return f.run(ctx, job)
}

func (f *fakeRunner) GiveUp(_ context.Context, _ store.Job, cause error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gaveUp = append(f.gaveUp, cause)
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func setup(t *testing.T, jobs int) (*store.Store, *syncBuf) {
	t.Helper()
	st := store.New(dbtest.New(t))
	for i := range jobs {
		sha := strings.Repeat("0", 39) + string(rune('1'+i))
		if _, _, err := st.CreateDeployment(context.Background(), store.NewDeployment{
			InstallationID: 1, AccountLogin: "o", Repo: store.Repo{ID: 1, FullName: "o/r"},
			CommitSHA: sha, Branch: "main", Pusher: store.System,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return st, &syncBuf{}
}

func pool(st *store.Store, r Runner, logs *syncBuf) *Pool {
	return &Pool{Queue: st, Runners: map[string]Runner{store.JobBuild: r}, Log: obs.NewLogger(logs), Name: "test", Workers: 2,
		Lease: time.Second, Heartbeat: 100 * time.Millisecond, Idle: 10 * time.Millisecond}
}

func jobState(t *testing.T, st *store.Store) (status string, attempts int) {
	t.Helper()
	if err := st.Pool.QueryRow(context.Background(), "SELECT status, attempts FROM jobs ORDER BY id LIMIT 1").
		Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	return status, attempts
}

// runUntil runs the pool until cond holds, then stops it.
func runUntil(t *testing.T, p *Pool, cond func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("condition never held")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
}

func TestJobCompletes(t *testing.T) {
	st, logs := setup(t, 1)
	r := &fakeRunner{run: func(context.Context, store.Job) error { return nil }}
	runUntil(t, pool(st, r, logs), func() bool { s, _ := jobState(t, st); return s == "done" })
	if r.runs.Load() != 1 || !strings.Contains(logs.String(), `"msg":"job done"`) {
		t.Fatalf("runs = %d\n%s", r.runs.Load(), logs)
	}
}

func TestFailedJobIsRetriedWithBackoff(t *testing.T) {
	st, logs := setup(t, 1)
	r := &fakeRunner{run: func(context.Context, store.Job) error { return errors.New("throttled") }}
	runUntil(t, pool(st, r, logs), func() bool { return strings.Contains(logs.String(), "will retry") })
	status, attempts := jobState(t, st)
	var wait time.Duration
	_ = st.Pool.QueryRow(context.Background(), "SELECT run_after - now() FROM jobs").Scan(&wait)
	if status != "queued" || attempts != 1 || wait < 20*time.Second {
		t.Fatalf("status=%s attempts=%d retry in %v", status, attempts, wait)
	}
}

func TestGiveUpAfterLastAttempt(t *testing.T) {
	st, logs := setup(t, 1)
	_, _ = st.Pool.Exec(context.Background(), "UPDATE jobs SET max_attempts = 1")
	r := &fakeRunner{run: func(context.Context, store.Job) error { return errors.New("throttled") }}
	runUntil(t, pool(st, r, logs), func() bool { r.mu.Lock(); defer r.mu.Unlock(); return len(r.gaveUp) == 1 })
	if s, _ := jobState(t, st); s != "failed" {
		t.Fatalf("status = %s", s)
	}
}

func TestShutdownReleasesJob(t *testing.T) {
	st, logs := setup(t, 1)
	r := &fakeRunner{run: func(ctx context.Context, _ store.Job) error { <-ctx.Done(); return ctx.Err() },
		running: make(chan struct{}, 1)}
	runUntil(t, pool(st, r, logs), func() bool { return r.runs.Load() == 1 })
	status, attempts := jobState(t, st)
	if status != "queued" || attempts != 0 {
		t.Fatalf("after shutdown: status=%s attempts=%d", status, attempts)
	}
	if !strings.Contains(logs.String(), "released for shutdown") {
		t.Fatal("release not logged")
	}
}

func TestHeartbeatKeepsLongJobWithOneWorker(t *testing.T) {
	st, logs := setup(t, 1)
	r := &fakeRunner{run: func(ctx context.Context, _ store.Job) error {
		time.Sleep(2500 * time.Millisecond) // longer than the 1s lease
		return ctx.Err()
	}}
	p := pool(st, r, logs)
	p.Workers = 4
	runUntil(t, p, func() bool { s, _ := jobState(t, st); return s == "done" })
	if n := r.runs.Load(); n != 1 {
		t.Fatalf("job ran %d times, want 1", n)
	}
}

func TestLostLeaseCancelsJob(t *testing.T) {
	st, logs := setup(t, 1)
	r := &fakeRunner{run: func(ctx context.Context, _ store.Job) error {
		_, _ = st.Pool.Exec(context.Background(), "UPDATE jobs SET locked_by = 'thief'")
		<-ctx.Done()
		return ctx.Err()
	}}
	p := pool(st, r, logs)
	p.Workers = 1
	runUntil(t, p, func() bool {
		return strings.Contains(logs.String(), `"msg":"job lease lost; another worker owns it now"`)
	})
	if s, _ := jobState(t, st); s != "running" {
		t.Fatalf("status = %s; the thief still owns it", s)
	}
}

func TestUnknownKindFailsWithoutRetry(t *testing.T) {
	st, logs := setup(t, 1)
	_, _ = st.Pool.Exec(context.Background(), "UPDATE jobs SET kind = 'mystery'")
	r := &fakeRunner{run: func(context.Context, store.Job) error { return nil }}
	runUntil(t, pool(st, r, logs), func() bool { s, _ := jobState(t, st); return s == "failed" })
	if r.runs.Load() != 0 {
		t.Fatal("runner called for unknown kind")
	}
}

func TestBackoff(t *testing.T) {
	for attempt, want := range map[int]time.Duration{1: 30 * time.Second, 2: 2 * time.Minute, 3: 5 * time.Minute} {
		if got := Backoff(attempt); got != want {
			t.Errorf("Backoff(%d) = %v, want %v", attempt, got, want)
		}
	}
}

func TestJobsRouteByKind(t *testing.T) {
	st, logs := setup(t, 1) // queues a build job
	ctx := context.Background()
	_, _ = st.SetDeploymentStatus(ctx, 1, store.StatusChange{To: store.StatusBuilding})
	_, _ = st.SetDeploymentStatus(ctx, 1, store.StatusChange{To: store.StatusBuilt}) // queues a deploy job
	var mu sync.Mutex
	seen := map[string]int{}
	runner := func(kind string) *fakeRunner {
		return &fakeRunner{run: func(_ context.Context, j store.Job) error {
			mu.Lock()
			defer mu.Unlock()
			if j.Kind == kind {
				seen[kind]++
			} else {
				seen["wrong runner for "+j.Kind]++
			}
			return nil
		}}
	}
	p := pool(st, nil, logs)
	p.Runners = map[string]Runner{store.JobBuild: runner(store.JobBuild), store.JobDeploy: runner(store.JobDeploy)}
	runUntil(t, p, func() bool { mu.Lock(); defer mu.Unlock(); return len(seen) >= 2 })
	if seen[store.JobBuild] != 1 || seen[store.JobDeploy] != 1 || len(seen) != 2 {
		t.Fatalf("seen = %v", seen)
	}
}
