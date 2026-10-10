package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/config"
	"github.com/Arpit529Srivastava/Foyr/internal/github"
	"github.com/Arpit529Srivastava/Foyr/internal/health"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
)

// newServer builds foyrd's routes.
func newServer(cfg *config.Config, log *slog.Logger) http.Handler {
	app := &github.App{ID: cfg.GitHubAppID, Key: cfg.GitHubPrivateKey, BaseURL: cfg.GitHubAPIURL}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health.Handler)
	mux.Handle("POST /webhooks/github", &github.Handler{
		Secret:    cfg.GitHubWebhookSecret,
		OnPush:    onPush(app),
		OnInstall: onInstall,
	})
	return obs.RequestID(log, mux)
}

// onPush logs the push and, in the background, proves the App can read that commit.
// Step 6 replaces the background check with queuing a build.
func onPush(app *github.App) func(context.Context, string, github.PushEvent) {
	return func(ctx context.Context, _ string, e github.PushEvent) {
		log := obs.Logger(ctx).With(
			"repo", e.Repository.FullName, "branch", e.Branch(), "commit", e.After, "installation", e.Installation.ID)
		switch {
		case e.Deleted:
			log.Info("push skipped", "reason", "branch deleted")
			return
		case e.Branch() == "":
			log.Info("push skipped", "reason", "not a branch", "ref", e.Ref)
			return
		}
		log.Info("push received")

		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			tok, err := app.InstallationToken(ctx, e.Installation.ID)
			if err != nil {
				log.Error("installation token failed", "err", err)
				return
			}
			if err := app.CheckCommit(ctx, tok, e.Repository.FullName, e.After); err != nil {
				log.Error("repo access failed", "err", err)
				return
			}
			log.Info("repo access ok", "token", tok, "token_expires_at", tok.ExpiresAt)
		}()
	}
}

// onInstall logs which repos an installation added or removed; step 6 stores them.
func onInstall(ctx context.Context, _ string, e github.InstallationEvent) {
	names := func(rs []github.RepoRef) []string {
		out := make([]string, 0, len(rs))
		for _, r := range rs {
			out = append(out, r.FullName)
		}
		return out
	}
	obs.Logger(ctx).Info("installation changed",
		"action", e.Action, "installation", e.Installation.ID, "account", e.Installation.Account.Login,
		"repos", names(append(e.Repositories, e.RepositoriesAdded...)), "removed", names(e.RepositoriesRemoved))
}
