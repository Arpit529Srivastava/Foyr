package run

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/db/dbtest"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

type fakePlatform struct {
	mu       sync.Mutex
	polls    [][]Task // returned in order; the last repeats
	gets     int
	startErr error
	stopErr  error
	specs    []TaskSpec
	services []ServiceSpec
	stopped  []string
}

func (f *fakePlatform) RegisterTask(_ context.Context, s TaskSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.specs = append(f.specs, s)
	return "arn:aws:ecs:task-definition/" + s.Family + ":1", nil
}

func (f *fakePlatform) StartService(_ context.Context, s ServiceSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	f.services = append(f.services, s)
	return nil
}

func (f *fakePlatform) Tasks(context.Context, string) ([]Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := min(f.gets, len(f.polls)-1)
	f.gets++
	return f.polls[i], nil
}

func (f *fakePlatform) StopService(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, name)
	return f.stopErr
}

type fakeLogs struct{}

func (fakeLogs) Tail(context.Context, string, string, int) ([]string, error) {
	return []string{"Error: Cannot find module 'next'"}, nil
}

type rig struct {
	r   *Runner
	st  *store.Store
	pf  *fakePlatform
	ctx context.Context
}

func newRig(t *testing.T, polls ...[]Task) *rig {
	t.Helper()
	st := store.New(dbtest.New(t))
	pf := &fakePlatform{polls: polls}
	return &rig{
		st: st, pf: pf,
		ctx: obs.WithLogger(context.Background(), obs.NewLogger(&bytes.Buffer{})),
		r: &Runner{Store: st, Platform: pf, Logs: fakeLogs{}, LogGroup: "/foyr/apps", Region: "ap-south-1",
			Poll: time.Millisecond, StartTimeout: time.Second},
	}
}

// built creates a deployment for commit n that has finished building.
func (rg *rig) built(t *testing.T, n int) store.Deployment {
	t.Helper()
	ctx := context.Background()
	d, _, err := rg.st.CreateDeployment(ctx, store.NewDeployment{InstallationID: 1, AccountLogin: "o",
		Repo: store.Repo{ID: 10, FullName: "o/r"}, CommitSHA: strings.Repeat(strconv.Itoa(n), 40), Branch: "main",
		Pusher: store.System})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = rg.st.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusBuilding})
	if d, err = rg.st.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusBuilt,
		Image: "123.dkr.ecr.ap-south-1.amazonaws.com/apps/1:d" + strconv.FormatInt(d.ID, 10)}); err != nil {
		t.Fatal(err)
	}
	return d
}

func (rg *rig) run(t *testing.T, d store.Deployment) store.Deployment {
	t.Helper()
	if err := rg.r.Run(rg.ctx, store.Job{ID: 1, Kind: store.JobDeploy, DeploymentID: d.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := rg.st.GetDeployment(context.Background(), d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (rg *rig) steps(t *testing.T, id int64) map[string]store.Step {
	t.Helper()
	steps, err := rg.st.Steps(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]store.Step{}
	for _, s := range steps {
		out[s.Name] = s
	}
	return out
}

func started(sec int) *time.Time {
	t := time.Now().Add(time.Duration(sec) * time.Second)
	return &t
}

var (
	provisioning = []Task{{ID: "t1", Status: "PROVISIONING", Health: "UNKNOWN"}}
	running      = []Task{{ID: "t1", Status: "RUNNING", Health: "UNKNOWN", StartedAt: started(-2)}}
	healthy      = []Task{{ID: "t1", Status: "RUNNING", Health: "HEALTHY", StartedAt: started(-2)}}
	exit1        = int32(1)
	crashed      = []Task{{ID: "t1", Status: "STOPPED", StoppedReason: "Essential container in task exited", ExitCode: &exit1}}
	pullFailed   = []Task{{ID: "t1", Status: "STOPPED", StoppedReason: "CannotPullContainerError: image not found"}}
)

func TestDeployGoesLive(t *testing.T) {
	rg := newRig(t, provisioning, running, healthy)
	rg.r.AuthURL = "https://auth.example.test"
	d := rg.run(t, rg.built(t, 1))
	if d.Status != store.StatusLive || d.ECSService != "d-1" || d.TaskDefinition != "arn:aws:ecs:task-definition/app-1:1" || d.LiveAt == nil {
		t.Fatalf("deployment = %+v", d)
	}
	spec := rg.pf.specs[0]
	if spec.Family != "app-1" || spec.Image != d.ImageURI || spec.Env["PORT"] != "3000" || spec.LogPrefix != "app-1" {
		t.Fatalf("task spec = %+v", spec)
	}
	app, err := rg.st.GetApp(context.Background(), d.AppID)
	if err != nil || app.Slug == "" {
		t.Fatalf("app = %+v, err = %v", app, err)
	}
	if spec.Env["FOYR_APP_SLUG"] != app.Slug || spec.Env["FOYR_AUTH_URL"] != "https://auth.example.test" {
		t.Fatalf("env = %v, want slug %q and the auth URL", spec.Env, app.Slug)
	}
	if s := rg.pf.services[0]; s.Name != "d-1" || s.AppID != 1 || s.DeploymentID != 1 {
		t.Fatalf("service spec = %+v", s)
	}
	steps := rg.steps(t, d.ID)
	if steps["Start"].Status != store.StepSucceeded || steps["Health check"].Status != store.StepSucceeded ||
		!strings.Contains(steps["Health check"].LogRef, "log-events/app-1$252Fapp$252Ft1") {
		t.Fatalf("steps = %+v", steps)
	}
	if a, _ := rg.st.GetApp(context.Background(), d.AppID); a.LiveDeployment != d.ID {
		t.Fatalf("app live deployment = %d", a.LiveDeployment)
	}
}

func TestAppEnv(t *testing.T) {
	for _, c := range []struct {
		name, slug, auth string
		want             map[string]string
	}{
		{"with auth", "shop", "https://auth.example.test", map[string]string{"PORT": "3000", "NODE_ENV": "production",
			"FOYR_APP_SLUG": "shop", "FOYR_AUTH_URL": "https://auth.example.test"}},
		{"without auth", "shop", "", map[string]string{"PORT": "3000", "NODE_ENV": "production", "FOYR_APP_SLUG": "shop"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := appEnv(c.slug, c.auth)
			if len(got) != len(c.want) {
				t.Fatalf("env = %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Fatalf("env[%s] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestDeployFailsWithoutASlug(t *testing.T) {
	rg := newRig(t, healthy)
	d := rg.built(t, 1)
	if _, err := rg.st.Pool.Exec(context.Background(), "UPDATE apps SET slug = NULL WHERE id = $1", d.AppID); err != nil {
		t.Fatal(err)
	}
	got := rg.run(t, d)
	if got.Status != store.StatusFailed || got.FailureStep != "Start" {
		t.Fatalf("deployment = %+v", got)
	}
	if len(rg.pf.specs) != 0 {
		t.Fatal("no task definition may be registered without a slug")
	}
}

func TestSecondDeploySupersedesFirst(t *testing.T) {
	rg := newRig(t, healthy)
	first := rg.run(t, rg.built(t, 1))
	second := rg.run(t, rg.built(t, 2))
	if second.Status != store.StatusLive {
		t.Fatalf("second = %s", second.Status)
	}
	if got, _ := rg.st.GetDeployment(context.Background(), first.ID); got.Status != store.StatusSuperseded {
		t.Fatalf("first = %s", got.Status)
	}
}

func TestCrashFailsAtHealthCheckAndKeepsPreviousLive(t *testing.T) {
	rg := newRig(t, healthy)
	first := rg.run(t, rg.built(t, 1))
	rg.pf.polls, rg.pf.gets = [][]Task{running, crashed}, 0
	d := rg.run(t, rg.built(t, 2))
	if d.Status != store.StatusFailed || d.FailureStep != "Health check" ||
		!strings.Contains(d.FailureReason, "exit code 1") || !strings.Contains(d.FailureReason, "Cannot find module") {
		t.Fatalf("deployment = %+v", d)
	}
	if got := rg.pf.stopped; len(got) != 1 || got[0] != "d-2" {
		t.Fatalf("stopped = %v, want the failed d-2 only", got)
	}
	if a, _ := rg.st.GetApp(context.Background(), d.AppID); a.LiveDeployment != first.ID {
		t.Fatal("previous live deployment was replaced by a failed one")
	}
}

func TestUnhealthyKillExplainsHealthCheck(t *testing.T) {
	code := int32(143)
	killed := []Task{{ID: "t1", Status: "STOPPED", StoppedReason: "Task failed container health checks", ExitCode: &code}}
	rg := newRig(t, running, killed)
	d := rg.run(t, rg.built(t, 1))
	if !strings.Contains(d.FailureReason, "/api/health, then /") || !strings.Contains(d.FailureReason, "port 3000") {
		t.Fatalf("reason should say what the health check needs: %s", d.FailureReason)
	}
}

func TestFailedDeployQueuesCleanup(t *testing.T) {
	rg := newRig(t, running, crashed)
	d := rg.run(t, rg.built(t, 1))
	var n int
	_ = rg.st.Pool.QueryRow(context.Background(),
		"SELECT count(*) FROM jobs WHERE kind = 'retire' AND deployment_id = $1", d.ID).Scan(&n)
	if n != 1 {
		t.Fatalf("retire jobs = %d, want 1 so leftovers (e.g. the Cloud Map name) are retried", n)
	}
}

func TestImageThatNeverStartsFailsAtStart(t *testing.T) {
	rg := newRig(t, provisioning, pullFailed)
	d := rg.run(t, rg.built(t, 1))
	if d.Status != store.StatusFailed || d.FailureStep != "Start" || !strings.Contains(d.FailureReason, "CannotPullContainerError") {
		t.Fatalf("deployment = %+v", d)
	}
}

func TestHealthCheckTimeout(t *testing.T) {
	rg := newRig(t, running)
	rg.r.StartTimeout = 30 * time.Millisecond
	d := rg.run(t, rg.built(t, 1))
	if d.Status != store.StatusFailed || d.FailureStep != "Health check" || !strings.Contains(d.FailureReason, "/api/health, then /") {
		t.Fatalf("deployment = %+v", d)
	}
	if rg.steps(t, d.ID)["Health check"].Message != "timed out" {
		t.Fatal("health step should say timed out")
	}
}

func TestDeployResumesExistingService(t *testing.T) {
	rg := newRig(t, healthy)
	d := rg.built(t, 1)
	ctx := context.Background()
	_, _ = rg.st.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusDeploying})
	_ = rg.st.SetRun(ctx, d.ID, "arn:td:1", "d-1")
	if got := rg.run(t, d); got.Status != store.StatusLive {
		t.Fatalf("status = %s", got.Status)
	}
	if len(rg.pf.specs) != 0 || len(rg.pf.services) != 0 {
		t.Fatal("resume must not register or start again")
	}
}

func TestPlatformErrorIsRetried(t *testing.T) {
	rg := newRig(t, healthy)
	d := rg.built(t, 1)
	rg.pf.startErr = errors.New("ThrottlingException")
	err := rg.r.Run(rg.ctx, store.Job{DeploymentID: d.ID})
	if err == nil || !strings.Contains(err.Error(), "start service") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := rg.st.GetDeployment(context.Background(), d.ID); got.Status != store.StatusDeploying || got.ECSService != "" {
		t.Fatalf("after error: %+v", got)
	}
	rg.pf.startErr = nil
	if got := rg.run(t, d); got.Status != store.StatusLive {
		t.Fatalf("retry: %s", got.Status)
	}
}

func TestDeploySkipsOtherStatuses(t *testing.T) {
	rg := newRig(t, healthy)
	d := rg.run(t, rg.built(t, 1)) // live
	rg.pf.specs = nil
	if err := rg.r.Run(rg.ctx, store.Job{DeploymentID: d.ID}); err != nil || len(rg.pf.specs) != 0 {
		t.Fatalf("err=%v specs=%v", err, rg.pf.specs)
	}
}

func TestGiveUpFailsAndCleansUp(t *testing.T) {
	rg := newRig(t, healthy)
	d := rg.built(t, 1)
	ctx := context.Background()
	_, _ = rg.st.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusDeploying})
	_ = rg.st.SetRun(ctx, d.ID, "arn:td:1", "d-1")
	rg.r.GiveUp(rg.ctx, store.Job{DeploymentID: d.ID}, errors.New("ThrottlingException"))
	got, _ := rg.st.GetDeployment(ctx, d.ID)
	if got.Status != store.StatusFailed || !strings.Contains(got.FailureReason, "ThrottlingException") || len(rg.pf.stopped) != 1 {
		t.Fatalf("deployment = %+v stopped=%v", got, rg.pf.stopped)
	}
}

func TestRetirer(t *testing.T) {
	rg := newRig(t, healthy)
	first := rg.run(t, rg.built(t, 1))
	rg.run(t, rg.built(t, 2))
	ret := &Retirer{Store: rg.st, Platform: rg.pf}
	if err := ret.Run(rg.ctx, store.Job{DeploymentID: first.ID}); err != nil {
		t.Fatal(err)
	}
	if got := rg.pf.stopped; len(got) != 1 || got[0] != "d-1" {
		t.Fatalf("stopped = %v", got)
	}
	var retired bool
	_ = rg.st.Pool.QueryRow(context.Background(), "SELECT retired_at IS NOT NULL FROM deployments WHERE id = $1", first.ID).Scan(&retired)
	if !retired {
		t.Fatal("retired_at not set")
	}
}

func TestRetirerNeverStopsLive(t *testing.T) {
	rg := newRig(t, healthy)
	live := rg.run(t, rg.built(t, 1))
	if err := (&Retirer{Store: rg.st, Platform: rg.pf}).Run(rg.ctx, store.Job{DeploymentID: live.ID}); err != nil {
		t.Fatal(err)
	}
	if len(rg.pf.stopped) != 0 {
		t.Fatal("retirer stopped the live deployment")
	}
}

func TestRetirerRetriesStopErrors(t *testing.T) {
	rg := newRig(t, healthy)
	first := rg.run(t, rg.built(t, 1))
	rg.run(t, rg.built(t, 2))
	rg.pf.stopErr = errors.New("ThrottlingException")
	if err := (&Retirer{Store: rg.st, Platform: rg.pf}).Run(rg.ctx, store.Job{DeploymentID: first.ID}); err == nil {
		t.Fatal("want error so the job retries")
	}
}

// TestHealthCheckCommand runs the real container command against a local server.
func TestHealthCheckCommand(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    bool
	}{
		{"api health ok", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/health" {
				http.NotFound(w, r)
			}
		}, true},
		{"only root ok", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
			}
		}, true},
		{"everything errors", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
			if got := healthCheckPasses(t, port); got != tt.want {
				t.Fatalf("healthy = %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("nothing listening", func(t *testing.T) {
		l, _ := net.Listen("tcp", "127.0.0.1:0")
		_, port, _ := net.SplitHostPort(l.Addr().String())
		l.Close()
		if healthCheckPasses(t, port) {
			t.Fatal("healthy with nothing listening")
		}
	})
}

// TestHealthCheckReachesAppBoundToTaskAddress covers Fargate setting HOSTNAME to the task's own
// name, so Next.js listens on that address and not on 127.0.0.1.
func TestHealthCheckReachesAppBoundToTaskAddress(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	l, err := net.Listen("tcp6", "[::1]:0") // reachable as "localhost", not as 127.0.0.1
	if err != nil {
		t.Skip("no IPv6 loopback")
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}
	go func() { _ = srv.Serve(l) }()
	defer srv.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	if !healthCheckPassesWith(t, port, "localhost") {
		t.Fatal("health check must also try the task's own address ($HOSTNAME)")
	}
}

func healthCheckPasses(t *testing.T, port string) bool {
	t.Helper()
	return healthCheckPassesWith(t, port, "")
}

func healthCheckPassesWith(t *testing.T, port, hostname string) bool {
	t.Helper()
	cmd := exec.Command("sh", "-c", HealthCheckCommand())
	cmd.Env = []string{"PORT=" + port, "HOSTNAME=" + hostname, "PATH=/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin"}
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return err == nil
}

func TestLogLink(t *testing.T) {
	r := &Runner{LogGroup: "/foyr/apps", Region: "ap-south-1"}
	want := "https://ap-south-1.console.aws.amazon.com/cloudwatch/home?region=ap-south-1#logsV2:log-groups/log-group/$252Ffoyr$252Fapps/log-events/app-7$252Fapp$252Fabc123"
	if got := r.logLink(7, "abc123"); got != want {
		t.Fatalf("logLink =\n%s\nwant\n%s", got, want)
	}
}
