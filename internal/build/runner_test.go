package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/db/dbtest"
	"github.com/Arpit529Srivastava/Foyr/internal/github"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

const secretToken = "ghs_supersecretvalue"

type fakeBuilds struct {
	mu       sync.Mutex
	startErr error
	started  []Request
	polls    []Build // returned in order; the last one repeats
	gets     int
}

func (f *fakeBuilds) Start(_ context.Context, r Request) (Build, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return Build{}, f.startErr
	}
	f.started = append(f.started, r)
	return Build{ID: "foyr-build:1", Status: "IN_PROGRESS", LogURL: "https://logs/1"}, nil
}

func (f *fakeBuilds) Get(_ context.Context, id string) (Build, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := min(f.gets, len(f.polls)-1)
	f.gets++
	b := f.polls[i]
	b.ID = id
	return b, nil
}

type fakeSecrets struct {
	mu      sync.Mutex
	values  map[string]string
	deleted []string
}

func (f *fakeSecrets) Put(_ context.Context, name, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[name] = value
	return nil
}

func (f *fakeSecrets) Delete(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.values, name)
	f.deleted = append(f.deleted, name)
	return nil
}

type fakeRegistry struct{ repos []string }

func (f *fakeRegistry) EnsureRepo(_ context.Context, name string) (string, error) {
	f.repos = append(f.repos, name)
	return "123456789012.dkr.ecr.ap-south-1.amazonaws.com/" + name, nil
}

type fakeLogs struct{ lines []string }

func (f fakeLogs) Tail(context.Context, string, string, int) ([]string, error) { return f.lines, nil }

type fakeTokens struct{ err error }

func (f fakeTokens) InstallationToken(context.Context, int64) (github.Token, error) {
	return github.Token{Value: secretToken, ExpiresAt: time.Now().Add(time.Hour)}, f.err
}

type rig struct {
	r       *Runner
	st      *store.Store
	builds  *fakeBuilds
	secrets *fakeSecrets
	logs    *bytes.Buffer
	ctx     context.Context
	job     store.Job
	d       store.Deployment
}

func newRig(t *testing.T, polls ...Build) *rig {
	t.Helper()
	st := store.New(dbtest.New(t))
	d, _, err := st.CreateDeployment(context.Background(), store.NewDeployment{
		InstallationID: 7, AccountLogin: "octo-dev",
		Repo:      store.Repo{ID: 70, FullName: "octo-dev/foyr-hello", DefaultBranch: "main"},
		CommitSHA: strings.Repeat("ab", 20), Branch: "main", Pusher: store.GitHubUser(1, "octo-dev"), TraceID: "trace-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	logs := &bytes.Buffer{}
	rg := &rig{
		st:      st,
		builds:  &fakeBuilds{polls: polls},
		secrets: &fakeSecrets{values: map[string]string{}},
		logs:    logs,
		ctx:     obs.WithLogger(context.Background(), obs.NewLogger(logs)),
		job:     store.Job{ID: 1, Kind: store.JobBuild, DeploymentID: d.ID, InstallationID: 7},
		d:       d,
	}
	rg.r = &Runner{Store: st, Builds: rg.builds, Registry: &fakeRegistry{}, Secrets: rg.secrets,
		Logs: fakeLogs{lines: []string{"> next build", "npm ERR! Missing script: build"}}, Tokens: fakeTokens{},
		Poll: time.Millisecond}
	return rg
}

func at(sec int) *time.Time {
	t := time.Date(2026, 10, 10, 12, 0, sec, 0, time.UTC)
	return &t
}

func phase(typ, status string, start, end int) Phase {
	p := Phase{Type: typ, Status: status, Start: at(start)}
	if end >= 0 {
		p.End = at(end)
	}
	return p
}

// prepared is every Prepare phase, finished successfully by second 10.
func prepared() []Phase {
	return []Phase{
		phase("SUBMITTED", "SUCCEEDED", 0, 1), phase("QUEUED", "SUCCEEDED", 1, 2),
		phase("PROVISIONING", "SUCCEEDED", 2, 8), phase("DOWNLOAD_SOURCE", "SUCCEEDED", 8, 9),
		phase("INSTALL", "SUCCEEDED", 9, 10),
	}
}

func succeeded() Build {
	return Build{Status: "SUCCEEDED", Phases: append(prepared(),
		phase("PRE_BUILD", "SUCCEEDED", 10, 13), phase("BUILD", "SUCCEEDED", 13, 100),
		phase("POST_BUILD", "SUCCEEDED", 100, 110), phase("COMPLETED", "", 110, -1))}
}

func failedInBuild() Build {
	b := phase("BUILD", "FAILED", 13, 60)
	b.Message = "COMMAND_EXECUTION_ERROR: Error while executing command: docker build. Reason: exit status 1"
	return Build{Status: "FAILED", LogGroup: "/foyr/builds", LogStream: "s1", Phases: append(prepared(),
		phase("PRE_BUILD", "SUCCEEDED", 10, 13), b, phase("COMPLETED", "", 60, -1))}
}

func (rg *rig) deployment(t *testing.T) store.Deployment {
	t.Helper()
	d, err := rg.st.GetDeployment(context.Background(), rg.d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (rg *rig) noTokenLeft(t *testing.T) {
	t.Helper()
	if len(rg.secrets.values) != 0 {
		t.Fatalf("build token still stored: %v", rg.secrets.values)
	}
	if strings.Contains(rg.logs.String(), secretToken) {
		t.Fatal("token leaked into logs")
	}
}

func TestRunSuccess(t *testing.T) {
	running := Build{Status: "IN_PROGRESS", Phases: append(prepared(), phase("PRE_BUILD", "", 10, -1))}
	rg := newRig(t, running, succeeded())
	if err := rg.r.Run(rg.ctx, rg.job); err != nil {
		t.Fatal(err)
	}
	d := rg.deployment(t)
	wantImage := "123456789012.dkr.ecr.ap-south-1.amazonaws.com/apps/1:d1-abababababab"
	if d.Status != store.StatusBuilt || d.ImageURI != wantImage || d.BuildID != "foyr-build:1" || d.LogURL != "https://logs/1" {
		t.Fatalf("deployment = %+v", d)
	}
	req := rg.builds.started[0]
	if req.RepoURL != "https://github.com/octo-dev/foyr-hello.git" || req.Commit != d.CommitSHA ||
		req.ImageURI != wantImage || req.TokenParam != "/foyr/builds/d-1/token" || req.DeploymentID != d.ID {
		t.Fatalf("build request = %+v", req)
	}
	steps, _ := rg.st.Steps(context.Background(), d.ID)
	var got []string
	for _, s := range steps {
		got = append(got, fmt.Sprintf("%s=%s/%v", s.Name, s.Status, s.Duration()))
	}
	want := "Prepare=succeeded/10s,Clone=succeeded/3s,Build image=succeeded/1m27s,Push image=succeeded/10s"
	if strings.Join(got, ",") != want {
		t.Fatalf("steps = %v\nwant %s", got, want)
	}
	rg.noTokenLeft(t)
}

func TestRunCustomerFailureIsRecordedNotRetried(t *testing.T) {
	rg := newRig(t, failedInBuild())
	if err := rg.r.Run(rg.ctx, rg.job); err != nil {
		t.Fatalf("customer failure must not be retried: %v", err)
	}
	d := rg.deployment(t)
	if d.Status != store.StatusFailed || d.FailureStep != "Build image" ||
		!strings.Contains(d.FailureReason, "exit status 1") || !strings.Contains(d.FailureReason, "Missing script: build") {
		t.Fatalf("deployment = %+v", d)
	}
	steps, _ := rg.st.Steps(context.Background(), d.ID)
	if steps[2].Status != store.StepFailed || steps[3].Status != store.StepSkipped {
		t.Fatalf("steps = %+v", steps)
	}
	rg.noTokenLeft(t)
}

func TestRunPlatformErrorIsRetriedThenResumes(t *testing.T) {
	rg := newRig(t, succeeded())
	rg.builds.startErr = errors.New("ThrottlingException: rate exceeded")
	if err := rg.r.Run(rg.ctx, rg.job); err == nil || !strings.Contains(err.Error(), "start build") {
		t.Fatalf("err = %v, want a retryable start error", err)
	}
	if d := rg.deployment(t); d.Status != store.StatusBuilding || d.BuildID != "" {
		t.Fatalf("after failed start: %+v", d)
	}
	rg.noTokenLeft(t)

	rg.builds.startErr = nil
	if err := rg.r.Run(rg.ctx, rg.job); err != nil {
		t.Fatal(err)
	}
	if d := rg.deployment(t); d.Status != store.StatusBuilt {
		t.Fatalf("retry: %+v", d)
	}
}

func TestRunResumesExistingBuild(t *testing.T) {
	rg := newRig(t, succeeded())
	ctx := context.Background()
	if _, err := rg.st.SetDeploymentStatus(ctx, rg.d.ID, store.StatusChange{To: store.StatusBuilding}); err != nil {
		t.Fatal(err)
	}
	if err := rg.st.SetBuild(ctx, rg.d.ID, "foyr-build:earlier", "https://logs/0", "repo:tag"); err != nil {
		t.Fatal(err)
	}
	if err := rg.r.Run(rg.ctx, rg.job); err != nil {
		t.Fatal(err)
	}
	if len(rg.builds.started) != 0 {
		t.Fatal("a second build was started instead of resuming")
	}
	if d := rg.deployment(t); d.Status != store.StatusBuilt || d.BuildID != "foyr-build:earlier" || d.ImageURI != "repo:tag" {
		t.Fatalf("deployment = %+v", d)
	}
}

func TestRunWithoutRepoAccessFailsAtOnce(t *testing.T) {
	rg := newRig(t, succeeded())
	rg.r.Tokens = fakeTokens{err: fmt.Errorf("github: %w", &github.StatusError{Code: 404})}
	if err := rg.r.Run(rg.ctx, rg.job); err != nil {
		t.Fatalf("err = %v, want recorded failure", err)
	}
	d := rg.deployment(t)
	if d.Status != store.StatusFailed || d.FailureStep != "Prepare" || !strings.Contains(d.FailureReason, "GitHub App") {
		t.Fatalf("deployment = %+v", d)
	}
	if len(rg.builds.started) != 0 {
		t.Fatal("build started without repo access")
	}
}

func TestRunSkipsFinishedDeployment(t *testing.T) {
	rg := newRig(t, succeeded())
	_, _ = rg.st.SetDeploymentStatus(context.Background(), rg.d.ID, store.StatusChange{To: store.StatusCanceled})
	if err := rg.r.Run(rg.ctx, rg.job); err != nil || len(rg.builds.started) != 0 {
		t.Fatalf("err=%v started=%d", err, len(rg.builds.started))
	}
}

func TestRunStopsOnShutdownAndKeepsTokenForResume(t *testing.T) {
	queued := Build{Status: "IN_PROGRESS", Phases: []Phase{phase("SUBMITTED", "SUCCEEDED", 0, 1), phase("QUEUED", "", 1, -1)}}
	rg := newRig(t, queued)
	ctx, cancel := context.WithCancel(rg.ctx)
	go func() {
		for rg.deployment(t).BuildID == "" {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	if err := rg.r.Run(ctx, rg.job); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, ok := rg.secrets.values["/foyr/builds/d-1/token"]; !ok {
		t.Fatal("token deleted before the build had cloned")
	}
}

func TestGiveUpMarksFailed(t *testing.T) {
	rg := newRig(t, succeeded())
	rg.r.GiveUp(rg.ctx, rg.job, errors.New("ThrottlingException"))
	d := rg.deployment(t)
	if d.Status != store.StatusFailed || !strings.Contains(d.FailureReason, "ThrottlingException") {
		t.Fatalf("deployment = %+v", d)
	}
}

func TestDescribeAndImageTag(t *testing.T) {
	for status, want := range map[string]string{
		"TIMED_OUT": "20 minutes", "STOPPED": "stopped", "FAULT": "internal error", "FAILED": "The build failed.",
	} {
		if got := describe(status, ""); !strings.Contains(got, want) {
			t.Errorf("describe(%s) = %q", status, got)
		}
	}
	if got := ImageTag(store.Deployment{ID: 42, CommitSHA: strings.Repeat("c", 40)}); got != "d42-cccccccccccc" {
		t.Errorf("ImageTag = %q", got)
	}
}
