package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Arpit529Srivastava/Foyr/internal/obs"
)

// maxBody matches GitHub's own webhook payload limit.
const maxBody = 25 << 20

// Repository is the part of a webhook's repository object foyrd uses.
type Repository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// Installation identifies which customer installation an event belongs to.
type Installation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
	} `json:"account"`
}

// PushEvent is a `push` webhook.
type PushEvent struct {
	Ref          string       `json:"ref"`
	After        string       `json:"after"`
	Deleted      bool         `json:"deleted"`
	Repository   Repository   `json:"repository"`
	Installation Installation `json:"installation"`
}

// Branch returns the branch name of a branch push, or "" for tags.
func (e PushEvent) Branch() string {
	b, ok := strings.CutPrefix(e.Ref, "refs/heads/")
	if !ok {
		return ""
	}
	return b
}

// RepoRef is a repository listed in installation events.
type RepoRef struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

// InstallationEvent is an `installation` or `installation_repositories` webhook.
type InstallationEvent struct {
	Action              string       `json:"action"`
	Installation        Installation `json:"installation"`
	Repositories        []RepoRef    `json:"repositories"`
	RepositoriesAdded   []RepoRef    `json:"repositories_added"`
	RepositoriesRemoved []RepoRef    `json:"repositories_removed"`
}

// Handler verifies and dispatches GitHub App webhooks.
// Callbacks run before the response is sent, so they must return quickly;
// ctx ends with the request, so background work must not inherit its cancellation.
type Handler struct {
	Secret    []byte
	OnPush    func(ctx context.Context, delivery string, e PushEvent)
	OnInstall func(ctx context.Context, delivery string, e InstallationEvent)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := obs.Logger(r.Context())
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !VerifySignature(h.Secret, body, r.Header.Get("X-Hub-Signature-256")) {
		log.Warn("webhook signature invalid")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	event, delivery := r.Header.Get("X-GitHub-Event"), r.Header.Get("X-GitHub-Delivery")
	log = log.With("event", event, "delivery", delivery)
	ctx := obs.WithLogger(r.Context(), log)

	switch event {
	case "ping":
		log.Info("webhook ping")
	case "push":
		var e PushEvent
		if err := json.Unmarshal(body, &e); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		if h.OnPush != nil {
			h.OnPush(ctx, delivery, e)
		}
	case "installation", "installation_repositories":
		var e InstallationEvent
		if err := json.Unmarshal(body, &e); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		if h.OnInstall != nil {
			h.OnInstall(ctx, delivery, e)
		}
	default:
		log.Info("webhook ignored")
	}
	w.WriteHeader(http.StatusAccepted)
}
