// Package founder serves a read-only page for the founder to watch apps and deployments (ADR-033).
// It is not the customer dashboard: one shared key, no accounts.
package founder

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// CookieName is host-only and HTTPS-only by the __Host- prefix rules.
const CookieName = "__Host-foyr_founder"

// SessionTTL is how long a login lasts.
const SessionTTL = 30 * 24 * time.Hour

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
}

// Handler serves /founder pages.
type Handler struct {
	Data       Data
	Key        string // FOYR_FOUNDER_KEY; empty turns the page off
	AppsDomain string // for links to live apps
	Now        func() time.Time
}

//go:embed templates/*.html
var files embed.FS

var pages = template.Must(template.New("").Funcs(template.FuncMap{
	"dur":     fmtDur,
	"since":   since,
	"short":   func(s string) string { return s[:min(len(s), 7)] },
	"first":   firstLine,
	"active":  active,
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
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// session is derived from the key, so the cookie never holds the key and a new key ends every session.
func (h *Handler) session() string {
	m := hmac.New(sha256.New, []byte(h.Key))
	m.Write([]byte("foyr founder session v1"))
	return hex.EncodeToString(m.Sum(nil))
}

func (h *Handler) enabled() bool { return len(h.Key) >= MinKeyLen }

func (h *Handler) signedIn(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(h.session())) == 1
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
	recent, err := h.Data.RecentDeployments(r.Context(), 100)
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
	key := r.PostFormValue("key")
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(key)), []byte(h.Key)) != 1 {
		obs.Logger(r.Context()).Warn("founder login refused")
		h.render(w, r, http.StatusUnauthorized, "login.html", map[string]any{"Error": "That key is not right."})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: h.session(), Path: "/", MaxAge: int(SessionTTL.Seconds()),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	obs.Logger(r.Context()).Info("founder signed in")
	http.Redirect(w, r, "/founder", http.StatusSeeOther)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
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
	h.render(w, r, http.StatusOK, name, data)
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]any) {
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Cache-Control", "no-store")
	hd.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Referrer-Policy", "no-referrer")
	hd.Set("X-Robots-Tag", "noindex, nofollow")
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

// active reports whether a deployment's status is still changing.
func active(status string) bool {
	switch status {
	case store.StatusQueued, store.StatusBuilding, store.StatusBuilt, store.StatusDeploying:
		return true
	}
	return false
}

// fmtDur prints 1m52s, 850ms or 3h04m.
func fmtDur(d time.Duration) string {
	switch {
	case d <= 0:
		return "-"
	case d < time.Second:
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	case d < time.Hour:
		return strings.Replace(d.Round(time.Second).String(), "m0s", "m", 1)
	default:
		return d.Round(time.Minute).String()
	}
}

// between is the time from a to b, or 0 when b is missing.
func between(a time.Time, b *time.Time) time.Duration {
	if b == nil {
		return 0
	}
	return b.Sub(a)
}

// since prints how long ago t was relative to now.
func since(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
