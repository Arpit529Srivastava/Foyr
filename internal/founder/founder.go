// Package founder serves a read-only page for the founder to watch apps and deployments (ADR-033).
// It is not the customer dashboard: one shared key, no accounts.
package founder

import (
	"context"
	"crypto/subtle"
	"embed"
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/mail"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
	"github.com/Arpit529Srivastava/Foyr/internal/ui"
	"github.com/Arpit529Srivastava/Foyr/internal/web"
)

// CookieName is host-only and HTTPS-only by the __Host- prefix rules.
const CookieName = "__Host-foyr_founder"

// SessionTTL is how long a login lasts; the server enforces it, not just the cookie.
const SessionTTL = 24 * time.Hour

// SidebarCookie remembers a hidden sidebar; it holds no secret.
const SidebarCookie = "foyr_sidebar"

// defaultFailDelay slows every wrong key.
const defaultFailDelay = 500 * time.Millisecond

// MinKeyLen keeps the key unguessable.
const MinKeyLen = 32

// Data is what the page reads; *store.Store satisfies it.
type Data interface {
	ListApps(ctx context.Context) ([]store.App, error)
	RecentDeployments(ctx context.Context, limit int) ([]store.AppDeployment, error)
	ListDeployments(ctx context.Context, appID, beforeID int64, limit int) ([]store.Deployment, error)
	GetDeployment(ctx context.Context, id int64) (store.Deployment, error)
	GetApp(ctx context.Context, id int64) (store.App, error)
	Steps(ctx context.Context, deploymentID int64) ([]store.Step, error)
	Timeline(ctx context.Context, deploymentID int64) ([]store.DeploymentEvent, error)
	ListAudit(ctx context.Context, f store.AuditFilter) ([]store.AuditEvent, error)

	// Who may open an account's dashboard (ADR-036).
	ListInstallations(ctx context.Context) ([]store.Installation, error)
	ListAccountMembers(ctx context.Context, installationID int64) ([]store.AccountMember, error)
	LinkAccountMember(ctx context.Context, installationID int64, email, role string, by store.Actor) (store.AccountMember, error)
	UnlinkAccountMember(ctx context.Context, installationID, userID int64, by store.Actor) error

	// Who may open an app (ADR-035).
	ListMembers(ctx context.Context, appID int64) ([]store.Member, error)
	ListInvites(ctx context.Context, appID int64) ([]store.Invite, error)
	CreateInvite(ctx context.Context, appID int64, email, role string, by store.Actor) (store.Invite, error)
	RevokeInvite(ctx context.Context, inviteID int64, by store.Actor) error
	RemoveMember(ctx context.Context, appID, userID int64, by store.Actor) error
	SetAccessMode(ctx context.Context, appID int64, mode string, by store.Actor) error
}

// Handler serves /founder pages.
type Handler struct {
	Data       Data
	Key        string      // FOYR_FOUNDER_KEY; empty turns the page off
	AppsDomain string      // for links to live apps
	AppsScheme string      // "https" unless developing locally
	Mail       mail.Sender // sends invites; nil saves them without an email
	SiteURL    string      // https://foyr.online, where the email banner image lives; empty in development
	Now        func() time.Time
	FailDelay  time.Duration // 0 means defaultFailDelay; negative turns it off (tests)
	lim        limiter
}

//go:embed templates/*.html
var files embed.FS

var pages = template.Must(template.New("").Funcs(template.FuncMap{
	"dur":     fmtDur,
	"since":   since,
	"short":   func(s string) string { return s[:min(len(s), 7)] },
	"first":   firstLine,
	"between": between,
	"clock":   func(t time.Time) string { return t.UTC().Format("Jan 2 15:04:05 UTC") },
	"delay":   func(i, step int) int { return min(i*step, 1200) },
	"add":     func(a, b int) int { return a + b },
}).ParseFS(files, "templates/*.html"))

// Register adds the routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /founder", h.home)
	mux.HandleFunc("POST /founder/login", h.login)
	mux.HandleFunc("POST /founder/logout", h.logout)
	mux.HandleFunc("GET /founder/deployments", h.guard(h.deployments))
	mux.HandleFunc("GET /founder/deployments/{id}", h.guard(h.deployment))
	mux.HandleFunc("GET /founder/activity", h.guard(h.activity))
	mux.HandleFunc("GET /founder/apps/{id}", h.guard(h.appPage))
	mux.HandleFunc("POST /founder/apps/{id}/invites", h.guardPost(h.invite))
	mux.HandleFunc("POST /founder/apps/{id}/invites/{iid}/revoke", h.guardPost(h.revokeInvite))
	mux.HandleFunc("POST /founder/apps/{id}/members/{uid}/remove", h.guardPost(h.removeMember))
	mux.HandleFunc("POST /founder/apps/{id}/access", h.guardPost(h.setAccess))
	mux.HandleFunc("GET /founder/accounts", h.guard(h.accounts))
	mux.HandleFunc("POST /founder/accounts/{id}/people", h.guardPost(h.linkPerson))
	mux.HandleFunc("POST /founder/accounts/{id}/people/{uid}/remove", h.guardPost(h.unlinkPerson))
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Handler) enabled() bool { return len(h.Key) >= MinKeyLen }

// SignedIn reports whether r carries a valid founder session, for other pages that borrow this sign-in.
func (h *Handler) SignedIn(r *http.Request) bool { return h.enabled() && h.signedIn(r) }

func (h *Handler) signedIn(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	return err == nil && h.validSession(c.Value, h.now())
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.NotFound(w, r)
		return
	}
	if !h.signedIn(r) {
		h.render(w, r, http.StatusOK, "login.html", map[string]any{})
		return
	}
	apps, err := h.Data.ListApps(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	recent, err := h.Data.RecentDeployments(r.Context(), recentLimit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	refresh := false
	for _, d := range recent {
		refresh = refresh || h.moving(d.Status, d.CreatedAt)
	}
	var latest *store.AppDeployment
	var pipeline []StepBar
	if len(recent) > 0 {
		latest = &recent[0]
		steps, err := h.Data.Steps(r.Context(), latest.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		pipeline = stepBars(steps, h.now())
	}
	h.page(w, r, "home.html", "overview", map[string]any{
		"Apps": apps, "Recent": recent[:min(len(recent), 8)], "Refresh": refresh,
		"O": summarize(recent, apps, h.now()), "Latest": latest, "Pipeline": pipeline,
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.NotFound(w, r)
		return
	}
	log := obs.Logger(r.Context())
	if !web.SameOrigin(r) {
		log.Warn("founder login refused: cross-site post", "origin", r.Header.Get("Origin"))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ip, now := web.ClientIP(r), h.now()
	if ok, wait := h.lim.begin(ip, now); !ok {
		log.Warn("founder login refused: rate limited", "ip", ip)
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		h.render(w, r, http.StatusTooManyRequests, "login.html", map[string]any{
			"Error": "Too many sign-in attempts. Try again in " + fmtDur(wait.Round(time.Second)+time.Second) + "."})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(r.PostFormValue("key"))), []byte(h.Key)) != 1 {
		log.Warn("founder login refused: wrong key", "ip", ip)
		delay := h.FailDelay
		if delay == 0 {
			delay = defaultFailDelay
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		h.render(w, r, http.StatusUnauthorized, "login.html", map[string]any{"Error": "That key is not right."})
		return
	}
	h.lim.forgive(ip, now)
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: h.newSession(now), Path: "/", MaxAge: int(SessionTTL.Seconds()),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	log.Info("founder signed in", "ip", ip)
	http.Redirect(w, r, "/founder", http.StatusSeeOther)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if !web.SameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true,
		SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/founder", http.StatusSeeOther)
}

// guard sends signed-out visitors to the sign-in page, and 404s when the page is off.
func (h *Handler) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.enabled() {
			http.NotFound(w, r)
			return
		}
		if !h.signedIn(r) {
			http.Redirect(w, r, "/founder", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// deployments lists every deployment, or one app's with ?app=<id>.
func (h *Handler) deployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data := map[string]any{}
	var rows []store.AppDeployment
	if v := r.URL.Query().Get("app"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			http.NotFound(w, r)
			return
		}
		app, err := h.Data.GetApp(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
		list, err := h.Data.ListDeployments(ctx, id, 0, 100)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		for _, d := range list {
			rows = append(rows, store.AppDeployment{Deployment: d, AppName: app.FullName, Slug: app.Slug})
		}
		data["App"] = app
	} else {
		list, err := h.Data.RecentDeployments(ctx, 100)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		rows = list
	}
	refresh := false
	for _, d := range rows {
		refresh = refresh || h.moving(d.Status, d.CreatedAt)
	}
	data["Rows"], data["Refresh"] = rows, refresh
	h.page(w, r, "deployments.html", "deployments", data)
}

// activity is the audit log: every change foyrd recorded, newest first.
func (h *Handler) activity(w http.ResponseWriter, r *http.Request) {
	events, err := h.Data.ListAudit(r.Context(), store.AuditFilter{Limit: 200})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, r, "activity.html", "activity", map[string]any{"Events": events})
}

func (h *Handler) deployment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	d, err := h.Data.GetDeployment(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	app, err := h.Data.GetApp(ctx, d.AppID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	steps, err := h.Data.Steps(ctx, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	events, err := h.Data.Timeline(ctx, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	audit, err := h.Data.ListAudit(ctx, store.AuditFilter{TargetType: "deployment", TargetID: strconv.FormatInt(id, 10), Limit: 100})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, r, "deployment.html", "deployments", map[string]any{
		"D": d, "App": app, "Steps": stepBars(steps, h.now()), "Events": events, "Audit": audit,
		"Refresh": h.moving(d.Status, d.CreatedAt),
	})
}

// page renders a signed-in page with the sidebar: its app list, the current section and the clock.
func (h *Handler) page(w http.ResponseWriter, r *http.Request, name, section string, data map[string]any) {
	if _, ok := data["Apps"]; !ok {
		apps, err := h.Data.ListApps(r.Context())
		if err != nil {
			h.fail(w, r, err)
			return
		}
		data["Apps"] = apps
	}
	data["Signed"], data["Section"], data["Domain"], data["Now"] = true, section, h.AppsDomain, h.now()
	if c, err := r.Cookie(SidebarCookie); err == nil && c.Value == "collapsed" {
		data["Collapsed"] = true
	}
	h.render(w, r, http.StatusOK, name, data)
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]any) {
	data["Nonce"] = ui.SecureHeaders(w)
	w.WriteHeader(status)
	if err := pages.ExecuteTemplate(w, name, data); err != nil {
		obs.Logger(r.Context()).Error("founder page render failed", "page", name, "err", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	obs.Logger(r.Context()).Error("founder page failed", "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// RefreshFor is how long after a push the page keeps refreshing itself while the deployment runs.
const RefreshFor = 30 * time.Minute

// moving reports whether to refresh: still running and recent, since old ones can be stuck from before deploys existed.
func (h *Handler) moving(status string, created time.Time) bool {
	return active(status) && h.now().Sub(created) < RefreshFor
}
