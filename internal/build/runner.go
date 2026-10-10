// Package build turns a queued deployment into a container image with CodeBuild (ADR-027).
package build

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/github"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// Phase is one CodeBuild phase.
type Phase struct {
	Type    string
	Status  string // empty while running
	Start   *time.Time
	End     *time.Time
	Message string
}

// Build is the state of one CodeBuild run.
type Build struct {
	ID        string
	Status    string // IN_PROGRESS, SUCCEEDED, FAILED, FAULT, TIMED_OUT, STOPPED
	Phases    []Phase
	LogURL    string
	LogGroup  string
	LogStream string
}

// Done reports whether the build has finished.
func (b Build) Done() bool { return b.Status != "" && b.Status != "IN_PROGRESS" }

// Request is what one build needs.
type Request struct {
	RepoURL      string
	Commit       string
	ImageURI     string
	TokenParam   string // SSM parameter holding the clone token
	DeploymentID int64
}

// Builds starts and inspects CodeBuild runs.
type Builds interface {
	Start(ctx context.Context, r Request) (Build, error)
	Get(ctx context.Context, id string) (Build, error)
}

// Registry makes sure an image repository exists and returns its URI.
type Registry interface {
	EnsureRepo(ctx context.Context, name string) (string, error)
}

// Secrets stores short-lived values for builds.
type Secrets interface {
	Put(ctx context.Context, name, value string) error
	Delete(ctx context.Context, name string) error
}

// Logs reads the end of a build log.
type Logs interface {
	Tail(ctx context.Context, group, stream string, n int) ([]string, error)
}

// Tokens mints installation tokens.
type Tokens interface {
	InstallationToken(ctx context.Context, installationID int64) (github.Token, error)
}

// tailLines is how many log lines a failure reason keeps.
const tailLines = 30

// Runner builds one deployment per job.
type Runner struct {
	Store    *store.Store
	Builds   Builds
	Registry Registry
	Secrets  Secrets
	Logs     Logs
	Tokens   Tokens
	Poll     time.Duration // how often to check a running build
}

// Run builds the deployment behind job and records every step. It returns an error only for
// platform problems worth retrying; a customer's failing build is recorded and returns nil.
// A job picked up again after a crash resumes the existing build instead of starting another.
func (r *Runner) Run(ctx context.Context, job store.Job) error {
	d, err := r.Store.GetDeployment(ctx, job.DeploymentID)
	if err != nil {
		return fmt.Errorf("load deployment: %w", err)
	}
	log := obs.Logger(ctx).With("deploy_id", d.ID, "app_id", d.AppID, "trace_id", d.TraceID, "job_id", job.ID)
	ctx = obs.WithLogger(ctx, log)
	switch d.Status {
	case store.StatusBuilt, store.StatusFailed, store.StatusCanceled:
		log.Info("deployment already finished", "status", d.Status)
		return nil
	}

	if d.BuildID != "" {
		log.Info("resuming build", "build_id", d.BuildID)
		return r.watch(ctx, d)
	}
	d, started, err := r.start(ctx, d)
	if !started {
		r.dropToken(ctx, d.ID)
		return err
	}
	return r.watch(ctx, d)
}

// dropToken deletes the build's clone token; deleting a missing token is fine.
func (r *Runner) dropToken(ctx context.Context, deploymentID int64) {
	if err := r.Secrets.Delete(context.WithoutCancel(ctx), tokenParam(deploymentID)); err != nil {
		obs.Logger(ctx).Warn("delete build token failed", "err", err)
	}
}

// start marks the deployment building, stores the clone token and starts CodeBuild.
// started is false when there is no build to watch: an error to retry, or a recorded failure.
func (r *Runner) start(ctx context.Context, d store.Deployment) (_ store.Deployment, started bool, err error) {
	if d.Status == store.StatusQueued {
		if d, err = r.Store.SetDeploymentStatus(ctx, d.ID, store.StatusChange{
			To: store.StatusBuilding, Actor: store.System, Message: "Build started"}); err != nil {
			return d, false, fmt.Errorf("mark building: %w", err)
		}
	}
	app, err := r.Store.GetApp(ctx, d.AppID)
	if err != nil {
		return d, false, fmt.Errorf("load app: %w", err)
	}

	tok, err := r.Tokens.InstallationToken(ctx, d.InstallationID)
	if github.NoAccess(err) {
		return d, false, r.fail(ctx, d, "Prepare",
			"Foyr can no longer read this repository. Check the GitHub App is still installed on it.")
	}
	if err != nil {
		return d, false, fmt.Errorf("installation token: %w", err)
	}
	if err := r.Secrets.Put(ctx, tokenParam(d.ID), tok.Value); err != nil {
		return d, false, fmt.Errorf("store build token: %w", err)
	}
	repo, err := r.Registry.EnsureRepo(ctx, "apps/"+strconv.FormatInt(app.ID, 10))
	if err != nil {
		return d, false, fmt.Errorf("image repository: %w", err)
	}
	image := repo + ":" + ImageTag(d)
	b, err := r.Builds.Start(ctx, Request{
		RepoURL:      "https://github.com/" + app.FullName + ".git",
		Commit:       d.CommitSHA,
		ImageURI:     image,
		TokenParam:   tokenParam(d.ID),
		DeploymentID: d.ID,
	})
	if err != nil {
		return d, false, fmt.Errorf("start build: %w", err)
	}
	// The build is running from here on, so a failure to save it is logged, not retried.
	if err := r.Store.SetBuild(ctx, d.ID, b.ID, b.LogURL, image); err != nil {
		obs.Logger(ctx).Error("save build id failed", "build_id", b.ID, "err", err)
	}
	d.BuildID, d.LogURL, d.ImageURI = b.ID, b.LogURL, image
	obs.Logger(ctx).Info("build started", "build_id", b.ID, "image", image)
	return d, true, nil
}

// watch polls the build until it finishes, recording steps on every poll.
func (r *Runner) watch(ctx context.Context, d store.Deployment) error {
	log := obs.Logger(ctx)
	poll := r.Poll
	if poll <= 0 {
		poll = 10 * time.Second
	}
	t := time.NewTicker(poll)
	defer t.Stop()
	tokenGone := false
	for {
		b, err := r.Builds.Get(ctx, d.BuildID)
		switch {
		case err != nil && ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			log.Warn("build status failed", "build_id", d.BuildID, "err", err)
		default:
			steps := Steps(b)
			if err := r.Store.SyncSteps(ctx, d.ID, steps); err != nil {
				log.Warn("save steps failed", "err", err)
			}
			// The token is only needed until the clone finishes.
			if !tokenGone && (b.Done() || cloneOver(steps)) {
				r.dropToken(ctx, d.ID)
				tokenGone = true
			}
			if b.Done() {
				return r.finish(ctx, d, b, steps)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// finish records the build result.
func (r *Runner) finish(ctx context.Context, d store.Deployment, b Build, steps []store.Step) error {
	log := obs.Logger(ctx)
	if b.Status == "SUCCEEDED" {
		if _, err := r.Store.SetDeploymentStatus(ctx, d.ID, store.StatusChange{
			To: store.StatusBuilt, Actor: store.System, Message: "Image built and pushed", Image: d.ImageURI}); err != nil {
			return fmt.Errorf("mark built: %w", err)
		}
		log.Info("build succeeded", "build_id", b.ID, "image", d.ImageURI)
		return nil
	}

	step, msg := failure(steps)
	if step == "" {
		step = "Build"
	}
	reason := describe(b.Status, msg)
	if lines, err := r.Logs.Tail(ctx, b.LogGroup, b.LogStream, tailLines); err != nil {
		log.Warn("read build log failed", "err", err)
	} else if len(lines) > 0 {
		reason += "\n\nLast lines of the build log:\n" + strings.Join(lines, "\n")
	}
	log.Info("build failed", "build_id", b.ID, "status", b.Status, "step", step)
	return r.fail(ctx, d, step, reason)
}

// fail marks the deployment failed for a reason the customer can act on; the job itself succeeded.
func (r *Runner) fail(ctx context.Context, d store.Deployment, step, reason string) error {
	if _, err := r.Store.SetDeploymentStatus(ctx, d.ID, store.StatusChange{
		To: store.StatusFailed, Actor: store.System, Message: "Build failed at " + step, Step: step, Reason: reason,
	}); err != nil && !errors.Is(err, store.ErrBadTransition) {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

// GiveUp marks the deployment failed after the job ran out of retries.
func (r *Runner) GiveUp(ctx context.Context, job store.Job, cause error) {
	d, err := r.Store.GetDeployment(ctx, job.DeploymentID)
	if err == nil {
		err = r.fail(ctx, d, "Prepare", "Foyr could not start this build after several tries ("+cause.Error()+
			"). This is on our side; push again or try later.")
	}
	if err != nil {
		obs.Logger(ctx).Error("give up failed", "job_id", job.ID, "err", err)
	}
}

// describe turns a CodeBuild status into a sentence.
func describe(status, msg string) string {
	var s string
	switch status {
	case "TIMED_OUT":
		s = "The build took longer than 20 minutes and was stopped."
	case "STOPPED":
		s = "The build was stopped."
	case "FAULT":
		s = "The build service had an internal error. Push again to retry."
	default:
		s = "The build failed."
	}
	if msg != "" {
		s += " " + msg
	}
	return s
}

// ImageTag is unique per deployment, so image tags never need to be overwritten.
func ImageTag(d store.Deployment) string {
	return fmt.Sprintf("d%d-%s", d.ID, d.CommitSHA[:12])
}

func tokenParam(deploymentID int64) string {
	return fmt.Sprintf("/foyr/builds/d-%d/token", deploymentID)
}
