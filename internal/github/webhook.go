package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

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

// User is the GitHub account that caused an event.
type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// Commit is the head commit of a push.
type Commit struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	URL     string `json:"url"`
}

// PushEvent is a `push` webhook.
type PushEvent struct {
	Ref          string       `json:"ref"`
	After        string       `json:"after"`
	Deleted      bool         `json:"deleted"`
	Repository   Repository   `json:"repository"`
	Installation Installation `json:"installation"`
	Sender       User         `json:"sender"`
	HeadCommit   *Commit      `json:"head_commit"`
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
	Private  bool   `json:"private"`
}

// InstallationEvent is an `installation` or `installation_repositories` webhook.
type InstallationEvent struct {
	Action              string       `json:"action"`
	Installation        Installation `json:"installation"`
	Repositories        []RepoRef    `json:"repositories"`
	RepositoriesAdded   []RepoRef    `json:"repositories_added"`
	RepositoriesRemoved []RepoRef    `json:"repositories_removed"`
	Sender              User         `json:"sender"`
}

// Outcome is what a callback did with an event: Result is "ok" or "ignored".
type Outcome struct {
	Result string
	Detail string
}

// Delivery describes one verified webhook after it was handled, for the delivery log (ADR-026).
type Delivery struct {
	ID             string
	Event          string
	Action         string
	InstallationID int64
	Repo           string
	Result         string // ok, ignored or error
	Detail         string
	Duration       time.Duration
}

// Handler verifies and dispatches GitHub App webhooks.
// Callbacks run before the response is sent, so they must return quickly. A callback error
// answers 500 so GitHub marks the delivery failed and it can be redelivered.
type Handler struct {
	Secret    []byte
	OnPush    func(ctx context.Context, delivery string, e PushEvent) (Outcome, error)
	OnInstall func(ctx context.Context, delivery string, e InstallationEvent) (Outcome, error)
	// Record, if set, receives every verified delivery once it is handled.
	Record func(ctx context.Context, d Delivery)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
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
	// Unverified requests are never stored, so a flood of forged calls cannot fill the database.
	if !VerifySignature(h.Secret, body, r.Header.Get("X-Hub-Signature-256")) {
		log.Warn("webhook signature invalid")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	event, delivery := r.Header.Get("X-GitHub-Event"), r.Header.Get("X-GitHub-Delivery")
	log = log.With("event", event, "delivery", delivery)
	ctx := obs.WithLogger(r.Context(), log)
	d := Delivery{ID: delivery, Event: event}
	out, err := h.dispatch(ctx, &d, body)

	status := http.StatusAccepted
	switch {
	case errors.Is(err, errBadPayload):
		status, d.Result, d.Detail = http.StatusBadRequest, "error", "invalid payload"
	case err != nil:
		log.Error("webhook failed", "err", err)
		status, d.Result, d.Detail = http.StatusInternalServerError, "error", err.Error()
	default:
		d.Result, d.Detail = out.Result, out.Detail
	}
	d.Duration = time.Since(start)
	if h.Record != nil {
		h.Record(context.WithoutCancel(ctx), d)
	}
	if status != http.StatusAccepted {
		http.Error(w, http.StatusText(status), status)
		return
	}
	w.WriteHeader(status)
}

var errBadPayload = errors.New("invalid payload")

// dispatch decodes the event, fills d's routing fields and runs the matching callback.
func (h *Handler) dispatch(ctx context.Context, d *Delivery, body []byte) (Outcome, error) {
	ignored := Outcome{Result: "ignored", Detail: "event not handled"}
	switch d.Event {
	case "ping":
		obs.Logger(ctx).Info("webhook ping")
		return Outcome{Result: "ok", Detail: "ping"}, nil
	case "push":
		var e PushEvent
		if err := json.Unmarshal(body, &e); err != nil {
			return Outcome{}, errBadPayload
		}
		d.InstallationID, d.Repo = e.Installation.ID, e.Repository.FullName
		if h.OnPush == nil {
			return ignored, nil
		}
		return h.OnPush(ctx, d.ID, e)
	case "installation", "installation_repositories":
		var e InstallationEvent
		if err := json.Unmarshal(body, &e); err != nil {
			return Outcome{}, errBadPayload
		}
		d.Action, d.InstallationID = e.Action, e.Installation.ID
		if h.OnInstall == nil {
			return ignored, nil
		}
		return h.OnInstall(ctx, d.ID, e)
	default:
		obs.Logger(ctx).Info("webhook ignored")
		return ignored, nil
	}
}
