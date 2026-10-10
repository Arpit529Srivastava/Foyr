// Package run starts a built image on ECS Fargate, waits for its health check and makes it live (ADR-029).
package run

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/build"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// AppPort is the port every app listens on, passed to it as PORT.
const AppPort = 3000

// HealthPaths are tried in order; the app is healthy when one returns 2xx.
var HealthPaths = []string{"/api/health", "/"}

// Task is one ECS task of a deployment's service.
type Task struct {
	ID            string
	Status        string // PROVISIONING, PENDING, RUNNING, STOPPED, ...
	Health        string // UNKNOWN, HEALTHY, UNHEALTHY
	StoppedReason string
	ExitCode      *int32
	StartedAt     *time.Time
}

// TaskSpec describes the container to run.
type TaskSpec struct {
	Family    string // app-<app id>
	Image     string
	LogPrefix string // app-<app id>
	Env       map[string]string
}

// ServiceSpec describes the ECS service for one deployment.
type ServiceSpec struct {
	Name           string // d-<deployment id>
	TaskDefinition string
	AppID          int64
	DeploymentID   int64
}

// Platform runs services; awsx implements it with ECS and Cloud Map.
type Platform interface {
	RegisterTask(ctx context.Context, s TaskSpec) (string, error)
	StartService(ctx context.Context, s ServiceSpec) error // safe to repeat
	Tasks(ctx context.Context, service string) ([]Task, error)
	StopService(ctx context.Context, service string) error // nil if already gone
}

// Runner handles deploy jobs.
type Runner struct {
	Store        *store.Store
	Platform     Platform
	Logs         build.Logs
	LogGroup     string // /foyr/apps
	Region       string
	Poll         time.Duration // default 5s
	StartTimeout time.Duration // default 6m
}

// ServiceName is the ECS service, and Cloud Map name, of a deployment.
func ServiceName(deploymentID int64) string { return "d-" + strconv.FormatInt(deploymentID, 10) }

// Run starts the deployment's image and waits until it is healthy (live) or has clearly failed.
// Platform errors are returned for retry; an app that crashes or never gets healthy is recorded.
func (r *Runner) Run(ctx context.Context, job store.Job) error {
	d, err := r.Store.GetDeployment(ctx, job.DeploymentID)
	if err != nil {
		return fmt.Errorf("load deployment: %w", err)
	}
	log := obs.Logger(ctx).With("app_id", d.AppID)
	ctx = obs.WithLogger(ctx, log)
	switch d.Status {
	case store.StatusBuilt:
		if d, err = r.Store.SetDeploymentStatus(ctx, d.ID, store.StatusChange{
			To: store.StatusDeploying, Actor: store.System, Message: "Starting the app"}); err != nil {
			return fmt.Errorf("mark deploying: %w", err)
		}
	case store.StatusDeploying:
	default:
		log.Info("deployment not waiting to deploy", "status", d.Status)
		return nil
	}
	if d.ImageURI == "" {
		return r.fail(ctx, d, "Start", "This deployment has no image to run.", "")
	}

	if d.ECSService == "" {
		td, err := r.Platform.RegisterTask(ctx, TaskSpec{
			Family:    "app-" + strconv.FormatInt(d.AppID, 10),
			Image:     d.ImageURI,
			LogPrefix: "app-" + strconv.FormatInt(d.AppID, 10),
			Env:       map[string]string{"PORT": strconv.Itoa(AppPort), "NODE_ENV": "production"},
		})
		if err != nil {
			return fmt.Errorf("register task definition: %w", err)
		}
		name := ServiceName(d.ID)
		if err := r.Platform.StartService(ctx, ServiceSpec{Name: name, TaskDefinition: td, AppID: d.AppID,
			DeploymentID: d.ID}); err != nil {
			return fmt.Errorf("start service: %w", err)
		}
		if err := r.Store.SetRun(ctx, d.ID, td, name); err != nil {
			log.Error("save service name failed", "service", name, "err", err)
		}
		d.TaskDefinition, d.ECSService = td, name
		log.Info("service started", "service", name, "task_definition", td)
	} else {
		log.Info("resuming deploy", "service", d.ECSService)
	}
	return r.watch(ctx, d)
}

// watch polls the service's tasks until one is healthy, one stops, or the start timeout passes.
func (r *Runner) watch(ctx context.Context, d store.Deployment) error {
	log := obs.Logger(ctx)
	poll, timeout := r.Poll, r.StartTimeout
	if poll <= 0 {
		poll = 5 * time.Second
	}
	if timeout <= 0 {
		timeout = 6 * time.Minute
	}
	began := time.Now()
	deadline := began.Add(timeout)
	start := store.Step{Name: "Start", Position: 5, Status: store.StepRunning, StartedAt: &began}
	health := store.Step{Name: "Health check", Position: 6, Status: store.StepPending}
	sync := func() {
		if err := r.Store.SyncSteps(ctx, d.ID, []store.Step{start, health}); err != nil {
			log.Warn("save steps failed", "err", err)
		}
	}
	sync()

	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		tasks, err := r.Platform.Tasks(ctx, d.ECSService)
		switch {
		case err != nil && ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			log.Warn("list tasks failed", "err", err)
		default:
			now := time.Now()
			if running := firstWith(tasks, func(t Task) bool { return t.Status == "RUNNING" }); running != nil &&
				start.Status == store.StepRunning {
				start.Status, start.FinishedAt = store.StepSucceeded, orNow(running.StartedAt, now)
				start.LogRef = r.logLink(d.AppID, running.ID)
				health.Status, health.StartedAt, health.LogRef = store.StepRunning, start.FinishedAt, start.LogRef
				sync()
			}
			if healthy := firstWith(tasks, func(t Task) bool { return t.Health == "HEALTHY" }); healthy != nil {
				if start.Status == store.StepRunning {
					start.Status, start.FinishedAt = store.StepSucceeded, orNow(healthy.StartedAt, now)
					health.StartedAt = start.FinishedAt
				}
				health.Status, health.FinishedAt = store.StepSucceeded, &now
				health.LogRef = r.logLink(d.AppID, healthy.ID)
				sync()
				return r.live(ctx, d)
			}
			if stopped := firstWith(tasks, func(t Task) bool { return t.Status == "STOPPED" }); stopped != nil {
				step := &start
				if start.Status == store.StepSucceeded {
					step = &health
				}
				step.Status, step.FinishedAt, step.Message = store.StepFailed, &now, stopped.StoppedReason
				sync()
				return r.fail(ctx, d, step.Name, stoppedReason(*stopped), r.tail(ctx, d.AppID, stopped.ID))
			}
			if now.After(deadline) {
				step := &start
				if start.Status == store.StepSucceeded {
					step = &health
				}
				step.Status, step.FinishedAt = store.StepFailed, &now
				step.Message = "timed out"
				sync()
				taskID := ""
				if len(tasks) > 0 {
					taskID = tasks[0].ID
				}
				return r.fail(ctx, d, step.Name, fmt.Sprintf("The app did not pass its health check within %s. "+
					"Foyr checks %s on the port in PORT (%d) and needs a 2xx answer.",
					timeout, strings.Join(HealthPaths, ", then "), AppPort), r.tail(ctx, d.AppID, taskID))
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// live makes the deployment live; MarkLive supersedes the previous one.
func (r *Runner) live(ctx context.Context, d store.Deployment) error {
	res, err := r.Store.MarkLive(ctx, d.ID, store.System)
	if err != nil {
		return fmt.Errorf("mark live: %w", err)
	}
	obs.Logger(ctx).Info("deployment live", "service", d.ECSService, "live", res.Live, "superseded", res.Superseded)
	return nil
}

// fail stops the new service, so nothing broken keeps running, and records why; the previous live one is untouched.
func (r *Runner) fail(ctx context.Context, d store.Deployment, step, reason, logTail string) error {
	if d.ECSService != "" {
		if err := r.Platform.StopService(context.WithoutCancel(ctx), d.ECSService); err != nil {
			obs.Logger(ctx).Warn("stop failed service", "service", d.ECSService, "err", err)
		}
	}
	if logTail != "" {
		reason += "\n\nLast lines of the app log:\n" + logTail
	}
	if _, err := r.Store.SetDeploymentStatus(ctx, d.ID, store.StatusChange{To: store.StatusFailed, Actor: store.System,
		Message: "Deploy failed at " + step + "; the previous version keeps serving", Step: step, Reason: reason}); err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	obs.Logger(ctx).Info("deploy failed", "step", step)
	return nil
}

// GiveUp records a deploy that ran out of retries and removes anything it started.
func (r *Runner) GiveUp(ctx context.Context, job store.Job, cause error) {
	d, err := r.Store.GetDeployment(ctx, job.DeploymentID)
	if err == nil {
		err = r.fail(ctx, d, "Start", "Foyr could not start this deployment after several tries ("+cause.Error()+
			"). This is on our side; push again or try later.", "")
	}
	if err != nil {
		obs.Logger(ctx).Error("give up failed", "job_id", job.ID, "err", err)
	}
}

// tail returns the last lines a task logged, or "".
func (r *Runner) tail(ctx context.Context, appID int64, taskID string) string {
	if taskID == "" || r.Logs == nil {
		return ""
	}
	lines, err := r.Logs.Tail(ctx, r.LogGroup, logStream(appID, taskID), 30)
	if err != nil {
		obs.Logger(ctx).Warn("read app log failed", "err", err)
		return ""
	}
	return strings.Join(lines, "\n")
}

// logLink opens a task's log stream in the CloudWatch console.
func (r *Runner) logLink(appID int64, taskID string) string {
	enc := func(s string) string { return strings.ReplaceAll(url.PathEscape(s), "%2F", "$252F") }
	return fmt.Sprintf("https://%s.console.aws.amazon.com/cloudwatch/home?region=%s#logsV2:log-groups/log-group/%s/log-events/%s",
		r.Region, r.Region, enc(r.LogGroup), enc(logStream(appID, taskID)))
}

// logStream is the awslogs stream name: prefix/container/task id.
func logStream(appID int64, taskID string) string {
	return fmt.Sprintf("app-%d/app/%s", appID, taskID)
}

func stoppedReason(t Task) string {
	s := "The app stopped before it was healthy"
	if t.StoppedReason != "" {
		s += ": " + t.StoppedReason
	}
	if t.ExitCode != nil {
		s += fmt.Sprintf(" (exit code %d)", *t.ExitCode)
	}
	return s + "."
}

func firstWith(tasks []Task, ok func(Task) bool) *Task {
	for i := range tasks {
		if ok(tasks[i]) {
			return &tasks[i]
		}
	}
	return nil
}

func orNow(t *time.Time, now time.Time) *time.Time {
	if t != nil {
		return t
	}
	return &now
}

// Retirer handles retire jobs: it removes a superseded deployment's service.
type Retirer struct {
	Store    *store.Store
	Platform Platform
}

// Run stops the service and records the retirement; errors are retried.
func (r *Retirer) Run(ctx context.Context, job store.Job) error {
	d, err := r.Store.GetDeployment(ctx, job.DeploymentID)
	if err != nil {
		return fmt.Errorf("load deployment: %w", err)
	}
	if d.Status == store.StatusLive {
		obs.Logger(ctx).Warn("refusing to retire the live deployment", "deploy_id", d.ID)
		return nil
	}
	if d.ECSService != "" {
		if err := r.Platform.StopService(ctx, d.ECSService); err != nil {
			return fmt.Errorf("stop service %s: %w", d.ECSService, err)
		}
	}
	if err := r.Store.MarkRetired(ctx, d.ID); err != nil {
		return fmt.Errorf("mark retired: %w", err)
	}
	obs.Logger(ctx).Info("deployment retired", "service", d.ECSService)
	return nil
}

// GiveUp logs; a service left running is visible in ECS and costs little.
func (r *Retirer) GiveUp(ctx context.Context, job store.Job, cause error) {
	obs.Logger(ctx).Error("retire gave up; service may still be running", "deploy_id", job.DeploymentID, "err", cause)
}

// HealthCheckCommand is the container health check: healthy when any HealthPaths answers 2xx,
// using whichever of wget, curl or node the image has.
func HealthCheckCommand() string {
	var b strings.Builder
	b.WriteString(`p="${PORT:-` + strconv.Itoa(AppPort) + `}"; for u in`)
	for _, path := range HealthPaths {
		b.WriteString(" " + path)
	}
	b.WriteString(`; do ( wget -q -O /dev/null "http://127.0.0.1:$p$u" || curl -fsS -o /dev/null "http://127.0.0.1:$p$u" || ` +
		`node -e "fetch('http://127.0.0.1:'+process.argv[1]).then(r=>process.exit(r.ok?0:1),()=>process.exit(1))" "$p$u" ) ` +
		`>/dev/null 2>&1 && exit 0; done; exit 1`)
	return b.String()
}
