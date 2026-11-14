// Package login serves magic-link sign-in on the auth host (ADR-035).
package login

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
	"github.com/Arpit529Srivastava/Foyr/internal/mail"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
	"github.com/Arpit529Srivastava/Foyr/internal/ui"
	"github.com/Arpit529Srivastava/Foyr/internal/web"
)

//go:embed templates/*.html
var files embed.FS

var pages = template.Must(template.New("").Funcs(template.FuncMap{"consoleHead": ui.ConsoleHead}).ParseFS(files, "templates/*.html"))

// Store is what login needs from the data layer.
type Store interface {
	StartLogin(ctx context.Context, slug, email string, now time.Time) (raw string, ok bool, err error)
	FinishLogin(ctx context.Context, raw string, now time.Time) (code, slug string, err error)
}

// Handler serves /login and /login/verify, only for requests addressed to the host of AuthURL.
type Handler struct {
	Store      Store
	Mail       mail.Sender
	AuthURL    string // https://auth.<PLATFORM_DOMAIN>, or http://localhost:<port> in development
	AppsDomain string // APPS_DOMAIN, with a port in development
	SiteURL    string // https://foyr.online, where the email banner image lives; empty in development
	DashURL    string // https://app.<PLATFORM_DOMAIN>, where dashboard SSO runs; empty turns the hop off
	Now        func() time.Time

	byIP, byEmail window
	sending       sync.WaitGroup
}

// Register adds the login routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", h.guard(h.form))
	mux.HandleFunc("POST /login", h.guard(h.submit))
	mux.HandleFunc("GET /login/verify", h.guard(h.verifyPage))
	mux.HandleFunc("POST /login/verify", h.guard(h.verify))
}

// Wait blocks until every email handed to the sender has been tried; for tests and shutdown.
func (h *Handler) Wait() { h.sending.Wait() }

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Handler) authURL() *url.URL {
	u, err := url.Parse(strings.TrimRight(h.AuthURL, "/"))
	if err != nil {
		return &url.URL{}
	}
	return u
}

// guard answers 404 unless the request is for the auth host, and checks same-origin on posts.
func (h *Handler) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Host, h.authURL().Host) {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			if !web.SameOrigin(r) {
				obs.Logger(r.Context()).Warn("login refused: cross-site post", "origin", r.Header.Get("Origin"))
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
		}
		next(w, r)
	}
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, name string, data map[string]any) {
	data["Nonce"] = ui.SecureHeaders(w)
	if name == "verify.html" {
		// Chrome applies form-action to the redirect that follows a post, and ours lands on the app host.
		csp := w.Header().Get("Content-Security-Policy")
		w.Header().Set("Content-Security-Policy", strings.Replace(csp, "form-action 'self'",
			"form-action 'self' "+h.authURL().Scheme+"://*."+h.AppsDomain, 1))
	}
	w.WriteHeader(status)
	if err := pages.ExecuteTemplate(w, name, data); err != nil {
		obs.Logger(r.Context()).Error("login page render failed", "page", name, "err", err)
	}
}

func (h *Handler) form(w http.ResponseWriter, r *http.Request) {
	app, ret := authn.CleanSlug(r.URL.Query().Get("app")), authn.SafeReturn(r.URL.Query().Get("return"))
	// A signed-in owner or admin of the app's account needs no email: ask the dashboard host, the only host
	// that can read the dashboard cookie, to hand over a code (ADR-039). sso=0 comes back from a failed hop.
	if h.DashURL != "" && app != "" && r.URL.Query().Get("sso") != "0" {
		w.Header().Set("Cache-Control", "no-store")
		dest := strings.TrimRight(h.DashURL, "/") + "/dashboard/sso?" + url.Values{"app": {app}, "return": {ret}}.Encode()
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}
	h.render(w, r, http.StatusOK, "page.html", map[string]any{"App": app, "Return": ret})
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	log := obs.Logger(r.Context())
	app, ret := authn.CleanSlug(r.PostFormValue("app")), authn.SafeReturn(r.PostFormValue("return"))
	email, ok := authn.NormalizeEmail(r.PostFormValue("email"))
	if !ok {
		h.render(w, r, http.StatusBadRequest, "page.html", map[string]any{
			"App": app, "Return": ret, "Error": "Enter a valid email address."})
		return
	}
	now := h.now()
	// Every outcome below renders the same page, so the answer never says who is a member.
	if !h.byIP.allow(web.ClientIP(r), MaxPostsPerIP, now) || !h.byEmail.allow(email, MaxEmailsPerAddress, now) {
		log.Warn("login refused: rate limited", "app", app)
		h.render(w, r, http.StatusOK, "sent.html", map[string]any{})
		return
	}
	raw, ok, err := h.Store.StartLogin(r.Context(), app, email, now)
	if err != nil {
		log.Error("login start failed", "err", err)
	} else if ok {
		h.sendLink(r.Context(), app, email, raw, ret)
	}
	h.render(w, r, http.StatusOK, "sent.html", map[string]any{})
}

// sendLink emails the link in the background, so a slow mail provider doesn't show in the response time
// of members only.
func (h *Handler) sendLink(ctx context.Context, slug, email, raw, ret string) {
	link := h.authURL().String() + "/login/verify?" + url.Values{"t": {raw}, "app": {slug}, "return": {ret}}.Encode()
	m := mail.LoginMessage(email, slug, link, h.SiteURL)
	log := obs.Logger(ctx)
	ctx = context.WithoutCancel(ctx)
	h.sending.Add(1)
	go func() {
		defer h.sending.Done()
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := h.Mail.Send(ctx, m); err != nil {
			log.Error("login email failed", "app", slug, "err", err)
		}
	}()
}

// verifyPage shows a button and never touches the token: mail scanners open links before people do.
func (h *Handler) verifyPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("t") == "" {
		h.expired(w, r, "", "/")
		return
	}
	h.render(w, r, http.StatusOK, "verify.html", map[string]any{
		"T": q.Get("t"), "App": authn.CleanSlug(q.Get("app")), "Return": authn.SafeReturn(q.Get("return")),
	})
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	app, ret := authn.CleanSlug(r.PostFormValue("app")), authn.SafeReturn(r.PostFormValue("return"))
	raw := r.PostFormValue("t")
	if raw == "" {
		h.expired(w, r, app, ret)
		return
	}
	code, slug, err := h.Store.FinishLogin(r.Context(), raw, h.now())
	if err != nil {
		if err != store.ErrInvalidToken {
			obs.Logger(r.Context()).Error("login finish failed", "err", err)
		}
		h.expired(w, r, app, ret)
		return
	}
	dest := url.URL{Scheme: h.authURL().Scheme, Host: slug + "." + h.AppsDomain, Path: "/__foyr/callback",
		RawQuery: url.Values{"code": {code}, "return": {ret}}.Encode()}
	http.Redirect(w, r, dest.String(), http.StatusSeeOther)
}

func (h *Handler) expired(w http.ResponseWriter, r *http.Request, app, ret string) {
	h.render(w, r, http.StatusBadRequest, "error.html", map[string]any{"App": app, "Return": ret})
}
