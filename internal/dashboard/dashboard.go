// Package dashboard is the customer dashboard: one GitHub account's apps, deployments, settings and access.
// Who says who is signed in: the founder sees every account, a customer only the accounts they are linked to.
package dashboard

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/mail"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
	"github.com/Arpit529Srivastava/Foyr/internal/ui"
	"github.com/Arpit529Srivastava/Foyr/internal/web"
)

// SidebarCookie remembers a hidden sidebar; it holds no secret.
const SidebarCookie = "foyr_dash_sidebar"

// historyLimit is how many deployments an app's pages read.
const historyLimit = 100

// Data is what the dashboard reads; *store.Store satisfies it.
type Data interface {
	ListInstallations(ctx context.Context) ([]store.Installation, error)
	ListApps(ctx context.Context) ([]store.App, error)
	ListDeployments(ctx context.Context, appID, beforeID int64, limit int) ([]store.Deployment, error)
	GetDeployment(ctx context.Context, id int64) (store.Deployment, error)
	Steps(ctx context.Context, deploymentID int64) ([]store.Step, error)
	Timeline(ctx context.Context, deploymentID int64) ([]store.DeploymentEvent, error)
	ListMembers(ctx context.Context, appID int64) ([]store.Member, error)
	ListInvites(ctx context.Context, appID int64) ([]store.Invite, error)
	CreateInvite(ctx context.Context, appID int64, email, role string, by store.Actor) (store.Invite, error)
	RevokeInvite(ctx context.Context, inviteID int64, by store.Actor) error
	RemoveMember(ctx context.Context, appID, userID int64, by store.Actor) error
	SetAccessMode(ctx context.Context, appID int64, mode string, by store.Actor) error
}

// Handler serves /dashboard pages.
type Handler struct {
	Data       Data
	Who        func(*http.Request) (Person, bool) // who is signed in, if anyone
	SignIn     string                             // where everyone else is sent
	AppsDomain string
	AppsScheme string      // "https" unless developing locally
	Mail       mail.Sender // sends invites and sign-in links; nil saves invites without an email
	SiteURL    string      // where the email banner image lives; empty in development
	Now        func() time.Time

	Auth    Auth   // customer sign-in; nil turns it off
	DashURL string // https://app.<PLATFORM_DOMAIN>, or http://localhost:<port> in development

	byIP, byEmail web.Window
	sending       sync.WaitGroup
}

// Tab is one section of an app's page.
type Tab struct {
	Path, Name string
	Preview    bool // drawn with sample data until its feature is built
}

// Tabs are an app's sections, in sidebar order.
var Tabs = []Tab{
	{"", "Overview", false},
	{"deployments", "Deployments", false},
	{"logs", "Logs", true},
	{"metrics", "Metrics", true},
	{"access", "Access", false},
	{"environment", "Environment", true},
	{"domains", "Domains", false},
	{"settings", "Settings", false},
}

//go:embed templates/*.html
var files embed.FS

var pages = template.Must(template.New("").Funcs(template.FuncMap{
	"dur":     ui.Dur,
	"since":   ui.Since,
	"between": ui.Between,
	"first":   ui.FirstLine,
	"short":   func(s string) string { return s[:min(len(s), 7)] },
	"initial": func(s string) string { return strings.ToUpper(s[:min(len(s), 1)]) },
	"clock":   func(t time.Time) string { return t.UTC().Format("Jan 2, 15:04 UTC") },
	"delay":   func(i, step int) int { return min(i*step, 900) },
	"row": func(i int, login string, r Row, now time.Time, many bool) map[string]any {
		return map[string]any{"I": i, "Login": login, "R": r, "Now": now, "Many": many}
	},
	"sample": sample,
	"div":    func(a, b int) int { return a / b },
}).ParseFS(files, "templates/*.html"))

// Register adds the routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.signinRoute(h.root))
	mux.HandleFunc("GET /dashboard/login", h.signinRoute(h.signinForm))
	mux.HandleFunc("POST /dashboard/login", h.signinRoute(h.signinSubmit))
	mux.HandleFunc("GET /dashboard/login/verify", h.signinRoute(h.verifyPage))
	mux.HandleFunc("POST /dashboard/login/verify", h.signinRoute(h.verify))
	mux.HandleFunc("POST /dashboard/logout", h.signinRoute(h.signOut))
	mux.HandleFunc("GET /dashboard", h.guard(h.start))
	mux.HandleFunc("GET /dashboard/{account}", h.guard(h.account))
	mux.HandleFunc("GET /dashboard/{account}/{app}", h.guard(h.app))
	mux.HandleFunc("GET /dashboard/{account}/{app}/{tab}", h.guard(h.app))
	mux.HandleFunc("GET /dashboard/{account}/{app}/deployments/{id}", h.guard(h.deployment))
	mux.HandleFunc("POST /dashboard/{account}/{app}/access/invites", h.guardPost(h.invite))
	mux.HandleFunc("POST /dashboard/{account}/{app}/access/invites/{iid}/revoke", h.guardPost(h.revokeInvite))
	mux.HandleFunc("POST /dashboard/{account}/{app}/access/members/{uid}/remove", h.guardPost(h.removeMember))
	mux.HandleFunc("POST /dashboard/{account}/{app}/access/mode", h.guardPost(h.setMode))
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Handler) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := h.person(r)
		if !ok {
			http.Redirect(w, r, h.signInURL(r), http.StatusSeeOther)
			return
		}
		next(w, withPerson(r, p))
	}
}

// guardPost is guard for form posts: same-origin only, small bodies.
func (h *Handler) guardPost(next http.HandlerFunc) http.HandlerFunc {
	return h.guard(func(w http.ResponseWriter, r *http.Request) {
		if !web.SameOrigin(r) {
			obs.Logger(r.Context()).Warn("dashboard post refused: cross-site", "origin", r.Header.Get("Origin"), "path", r.URL.Path)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		next(w, r)
	})
}

// scope is the account (and app) a request is about, with what the sidebar needs.
type scope struct {
	Accounts []store.Installation
	Account  store.Installation
	Apps     []store.App // this account's only
	App      *store.App
}

// load resolves {account} and, when named, {app}; ok is false after it has answered 404 or 500.
func (h *Handler) load(w http.ResponseWriter, r *http.Request, wantApp bool) (scope, bool) {
	var s scope
	accounts, err := h.Data.ListInstallations(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return s, false
	}
	// Only the person's own accounts exist as far as this request is concerned.
	me := personOf(r)
	accounts = slices.DeleteFunc(slices.Clone(accounts), func(a store.Installation) bool { return !me.sees(a.ID) })
	s.Accounts = accounts
	i := slices.IndexFunc(accounts, func(a store.Installation) bool { return strings.EqualFold(a.Login, r.PathValue("account")) })
	if i < 0 {
		h.notFound(w, r)
		return s, false
	}
	s.Account = accounts[i]
	all, err := h.Data.ListApps(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return s, false
	}
	for _, a := range all {
		if a.InstallationID == s.Account.ID {
			s.Apps = append(s.Apps, a)
		}
	}
	if wantApp {
		j := slices.IndexFunc(s.Apps, func(a store.App) bool { return a.Slug == r.PathValue("app") })
		if j < 0 {
			h.notFound(w, r)
			return s, false
		}
		s.App = &s.Apps[j]
	}
	return s, true
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.Data.ListInstallations(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	me := personOf(r)
	accounts = slices.DeleteFunc(slices.Clone(accounts), func(a store.Installation) bool { return !me.sees(a.ID) })
	if len(accounts) == 0 {
		h.render(w, r, http.StatusOK, "empty.html", map[string]any{})
		return
	}
	http.Redirect(w, r, "/dashboard/"+accounts[0].Login, http.StatusSeeOther)
}

// Card is one app on the account page.
type Card struct {
	store.App
	Latest *store.Deployment
}

func (h *Handler) account(w http.ResponseWriter, r *http.Request) {
	s, ok := h.load(w, r, false)
	if !ok {
		return
	}
	var cards []Card
	var recent []Row
	for _, a := range s.Apps {
		list, err := h.Data.ListDeployments(r.Context(), a.ID, 0, 5)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		c := Card{App: a}
		if len(list) > 0 {
			c.Latest = &list[0]
		}
		cards = append(cards, c)
		for _, d := range list {
			recent = append(recent, Row{Deployment: d, Slug: a.Slug, Current: d.ID == a.LiveDeployment})
		}
	}
	slices.SortFunc(recent, func(a, b Row) int { return b.CreatedAt.Compare(a.CreatedAt) })
	h.page(w, r, s, "account.html", "", map[string]any{"Cards": cards, "Recent": recent[:min(len(recent), 8)]})
}

// Row is one deployment in a list.
type Row struct {
	store.Deployment
	Slug    string
	Current bool // the one serving traffic now
}

// Stats are the numbers on an app's overview.
type Stats struct {
	Week       int
	Median     time.Duration
	SuccessPct int
	Finished   int
}

func stats(list []store.Deployment, now time.Time) Stats {
	var s Stats
	var times []time.Duration
	reached := 0
	for _, d := range list {
		if now.Sub(d.CreatedAt) < 7*24*time.Hour {
			s.Week++
		}
		if !ui.Active(d.Status) {
			s.Finished++
		}
		if d.LiveAt != nil {
			reached++
			times = append(times, d.LiveAt.Sub(d.CreatedAt))
		}
	}
	if s.Finished > 0 {
		s.SuccessPct = reached * 100 / s.Finished
	}
	if len(times) > 0 {
		slices.Sort(times)
		s.Median = times[len(times)/2]
	}
	return s
}

func (h *Handler) app(w http.ResponseWriter, r *http.Request) {
	tab := r.PathValue("tab")
	i := slices.IndexFunc(Tabs, func(t Tab) bool { return t.Path == tab })
	if i < 0 {
		h.notFound(w, r)
		return
	}
	s, ok := h.load(w, r, true)
	if !ok {
		return
	}
	if tab == "access" {
		h.accessPage(w, r, s, http.StatusOK, "")
		return
	}
	list, err := h.Data.ListDeployments(r.Context(), s.App.ID, 0, historyLimit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := map[string]any{"Stats": stats(list, h.now())}
	var live *store.Deployment
	for k := range list {
		if list[k].ID == s.App.LiveDeployment {
			live = &list[k]
		}
	}
	data["Live"] = live
	refresh := false
	for _, d := range list {
		refresh = refresh || (ui.Active(d.Status) && h.now().Sub(d.CreatedAt) < 30*time.Minute)
	}
	data["Refresh"] = refresh && (tab == "" || tab == "deployments")

	switch tab {
	case "":
		if len(list) > 0 {
			steps, err := h.Data.Steps(r.Context(), list[0].ID)
			if err != nil {
				h.fail(w, r, err)
				return
			}
			data["Latest"], data["Pipeline"] = &list[0], ui.StepBars(steps, h.now())
		}
		data["Rows"] = rows(list[:min(len(list), 5)], s.App)
	case "deployments":
		f := r.URL.Query().Get("status")
		var shown []store.Deployment
		for _, d := range list {
			if f == "" || d.Status == f || (f == "building" && ui.Active(d.Status)) {
				shown = append(shown, d)
			}
		}
		data["Rows"], data["Filter"] = rows(shown, s.App), f
	}
	h.page(w, r, s, strings.ToLower(Tabs[i].Name)+".html", Tabs[i].Path, data)
}

func rows(list []store.Deployment, a *store.App) []Row {
	out := make([]Row, 0, len(list))
	for _, d := range list {
		out = append(out, Row{Deployment: d, Slug: a.Slug, Current: d.ID == a.LiveDeployment})
	}
	return out
}

func (h *Handler) deployment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		h.notFound(w, r)
		return
	}
	s, ok := h.load(w, r, true)
	if !ok {
		return
	}
	ctx := r.Context()
	d, err := h.Data.GetDeployment(ctx, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && d.AppID != s.App.ID) {
		h.notFound(w, r)
		return
	}
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
	h.page(w, r, s, "deployment.html", "deployments", map[string]any{
		"D": d, "Current": d.ID == s.App.LiveDeployment, "Steps": ui.StepBars(steps, h.now()), "Events": events,
		"Hint": hint(d.FailureStep, d.FailureReason), "Refresh": ui.Active(d.Status) && h.now().Sub(d.CreatedAt) < 30*time.Minute,
	})
}

// hint says in plain words what to try after a failed step.
func hint(step, reason string) string {
	s := strings.ToLower(step + " " + reason)
	switch {
	case step == "" && reason == "":
		return ""
	case strings.Contains(s, "health"), strings.Contains(s, "start"):
		return "Foyr sends traffic to the port in $PORT (3000) and waits up to 6 minutes for /api/health or / to answer 2xx. Make sure your app reads PORT from the environment and listens on 0.0.0.0, not localhost."
	case strings.Contains(s, "build"):
		return "The image didn't build. Check the Dockerfile (or your build command) runs on a clean machine; the last lines of the build log are below."
	case strings.Contains(s, "clone"), strings.Contains(s, "prepare"):
		return "Foyr couldn't fetch the code. Check the Foyr GitHub App still has access to this repository, and that the commit still exists."
	case strings.Contains(s, "push"):
		return "The image built but couldn't be stored. This is on Foyr's side; push again, and it retries on its own."
	}
	return "Push a fix, or open the full log below to see the last thing that ran."
}

// page renders a signed-in page inside the frame.
func (h *Handler) page(w http.ResponseWriter, r *http.Request, s scope, name, tab string, data map[string]any) {
	h.pageStatus(w, r, http.StatusOK, s, name, tab, data)
}

func (h *Handler) pageStatus(w http.ResponseWriter, r *http.Request, status int, s scope, name, tab string, data map[string]any) {
	data["S"], data["Tabs"], data["Tab"], data["Domain"], data["Now"] = s, Tabs, tab, h.AppsDomain, h.now()
	if c, err := r.Cookie(SidebarCookie); err == nil && c.Value == "collapsed" {
		data["Collapsed"] = true
	}
	h.render(w, r, status, name, data)
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]any) {
	data["Founder"] = personOf(r).Founder
	data["Nonce"] = ui.SecureHeaders(w)
	w.WriteHeader(status)
	if err := pages.ExecuteTemplate(w, name, data); err != nil {
		obs.Logger(r.Context()).Error("dashboard render failed", "page", name, "err", err)
	}
}

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusNotFound, "notfound.html", map[string]any{})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	obs.Logger(r.Context()).Error("dashboard failed", "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// sample is a fixed, natural-looking series for preview charts.
func sample(n, seed int) []int {
	out := make([]int, n)
	x := uint32(seed*2654435761 + 1)
	for i := range out {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		base := 40 + 30*((i+seed)%7)/6
		out[i] = min(100, base+int(x%30))
	}
	return out
}
