package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Arpit529Srivastava/Foyr/internal/config"
	"github.com/Arpit529Srivastava/Foyr/internal/github"
	"github.com/Arpit529Srivastava/Foyr/internal/health"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// newServer builds foyrd's routes.
func newServer(cfg *config.Config, log *slog.Logger, st *store.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health.Handler)
	mux.Handle("POST /webhooks/github", &github.Handler{
		Secret:    cfg.GitHubWebhookSecret,
		OnPush:    onPush(st),
		OnInstall: onInstall(st),
		Record:    recordDelivery(st),
	})
	return obs.RequestID(log, mux)
}

// onPush queues a deployment for pushes to an app's default branch.
func onPush(st *store.Store) func(context.Context, string, github.PushEvent) (github.Outcome, error) {
	return func(ctx context.Context, delivery string, e github.PushEvent) (github.Outcome, error) {
		log := obs.Logger(ctx).With(
			"repo", e.Repository.FullName, "branch", e.Branch(), "commit", e.After, "installation", e.Installation.ID)
		skip := func(reason string) (github.Outcome, error) {
			log.Info("push skipped", "reason", reason)
			return github.Outcome{Result: "ignored", Detail: reason}, nil
		}
		switch {
		case e.Deleted:
			return skip("branch deleted")
		case e.Branch() == "":
			return skip("not a branch")
		case e.Repository.DefaultBranch != "" && e.Branch() != e.Repository.DefaultBranch:
			return skip("not the default branch")
		}

		n := store.NewDeployment{
			InstallationID: e.Installation.ID,
			AccountLogin:   owner(e.Repository.FullName),
			Repo: store.Repo{ID: e.Repository.ID, FullName: e.Repository.FullName,
				DefaultBranch: e.Repository.DefaultBranch, Private: e.Repository.Private},
			CommitSHA:  e.After,
			Branch:     e.Branch(),
			Pusher:     store.GitHubUser(e.Sender.ID, e.Sender.Login),
			DeliveryID: delivery,
			TraceID:    obs.RequestIDFrom(ctx),
		}
		if c := e.HeadCommit; c != nil {
			n.CommitMessage, n.CommitURL = c.Message, c.URL
		}
		d, created, err := st.CreateDeployment(ctx, n)
		if err != nil {
			return github.Outcome{}, fmt.Errorf("create deployment: %w", err)
		}
		log = log.With("deploy_id", d.ID, "app_id", d.AppID)
		if !created {
			log.Info("deployment exists", "status", d.Status)
			return github.Outcome{Result: "ignored", Detail: fmt.Sprintf("deployment %d already exists", d.ID)}, nil
		}
		log.Info("deployment queued")
		return github.Outcome{Result: "ok", Detail: fmt.Sprintf("deployment %d queued", d.ID)}, nil
	}
}

// onInstall stores installation changes and the apps they add or remove.
func onInstall(st *store.Store) func(context.Context, string, github.InstallationEvent) (github.Outcome, error) {
	return func(ctx context.Context, _ string, e github.InstallationEvent) (github.Outcome, error) {
		repos := func(rs []github.RepoRef) []store.Repo {
			out := make([]store.Repo, 0, len(rs))
			for _, r := range rs {
				out = append(out, store.Repo{ID: r.ID, FullName: r.FullName, Private: r.Private})
			}
			return out
		}
		c := store.InstallationChange{
			Action:         e.Action,
			InstallationID: e.Installation.ID,
			AccountLogin:   e.Installation.Account.Login,
			Actor:          store.GitHubUser(e.Sender.ID, e.Sender.Login),
			TraceID:        obs.RequestIDFrom(ctx),
			Added:          repos(append(e.Repositories, e.RepositoriesAdded...)),
			Removed:        repos(e.RepositoriesRemoved),
		}
		if err := st.SyncInstallation(ctx, c); err != nil {
			return github.Outcome{}, fmt.Errorf("sync installation: %w", err)
		}
		obs.Logger(ctx).Info("installation changed", "action", e.Action, "installation", e.Installation.ID,
			"account", c.AccountLogin, "added", len(c.Added), "removed", len(c.Removed))
		return github.Outcome{Result: "ok", Detail: "installation " + e.Action}, nil
	}
}

// recordDelivery saves every verified webhook; a failure here is logged, never returned to GitHub.
func recordDelivery(st *store.Store) func(context.Context, github.Delivery) {
	return func(ctx context.Context, d github.Delivery) {
		err := st.RecordDelivery(ctx, store.Delivery{ID: d.ID, Event: d.Event, Action: d.Action,
			InstallationID: d.InstallationID, RepoFullName: d.Repo, Result: d.Result, Detail: d.Detail,
			Duration: d.Duration, TraceID: obs.RequestIDFrom(ctx)})
		if err != nil {
			obs.Logger(ctx).Warn("record delivery failed", "err", err)
		}
	}
}

// owner returns the account part of "owner/repo".
func owner(fullName string) string {
	o, _, _ := strings.Cut(fullName, "/")
	return o
}
