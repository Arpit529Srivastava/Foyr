package dashboard

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
	"github.com/Arpit529Srivastava/Foyr/internal/mail"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
	"github.com/Arpit529Srivastava/Foyr/internal/web"
)

// CookieName is the customer session cookie: host-only and HTTPS-only by the __Host- prefix rules.
const CookieName = "__Host-foyr_dash"

// Limits per 15 minutes, as for app login (ADR-035): emails per address, and form posts per client IP.
const (
	rateWindow          = 15 * time.Minute
	maxEmailsPerAddress = 3
	maxPostsPerIP       = 10
	maxSSOPerUser       = 20 // dashboard SSO hops per person per window; each one stores a code and an audit row
)

// Auth is the dashboard sign-in the store provides.
type Auth interface {
	StartDashboardLogin(ctx context.Context, email string, now time.Time) (raw string, ok bool, err error)
	FinishDashboardLogin(ctx context.Context, raw string, now time.Time) (store.DashboardSession, error)
	DashboardUserBySession(ctx context.Context, raw string, now time.Time) (store.DashboardUser, error)
	RevokeDashboardSession(ctx context.Context, raw string) error
	IssueSSOCode(ctx context.Context, userID int64, slug string, now time.Time) (string, error)
}

// Wait blocks until sign-in emails being sent in the background are done (tests).
func (h *Handler) Wait() { h.sending.Wait() }

func (h *Handler) dashURL() *url.URL {
	u, err := url.Parse(strings.TrimRight(h.DashURL, "/"))
	if err != nil {
		return &url.URL{}
	}
	return u
}

// onDashHost reports whether r is for the customer dashboard's own host.
func (h *Handler) onDashHost(r *http.Request) bool {
	return h.Auth != nil && h.DashURL != "" && strings.EqualFold(r.Host, h.dashURL().Host)
}

// person says who is signed in: the founder first, else a customer with a valid session cookie.
func (h *Handler) person(r *http.Request) (Person, bool) {
	if h.Who != nil {
		if p, ok := h.Who(r); ok {
			return p, true
		}
	}
	if h.Auth == nil {
		return Person{}, false
	}
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return Person{}, false
	}
	u, err := h.Auth.DashboardUserBySession(r.Context(), c.Value, h.now())
	if err != nil {
		if err != store.ErrInvalidToken {
			obs.Logger(r.Context()).Error("dashboard session lookup failed", "err", err)
		}
		return Person{}, false
	}
	return PersonFromUser(u), true
}

// signInURL is where a signed-out visitor is sent.
func (h *Handler) signInURL(r *http.Request) string {
	if h.onDashHost(r) {
		return "/dashboard/login"
	}
	return h.SignIn
}

// signinRoute answers 404 unless sign-in is on and the request is for the dashboard host; posts must be same-origin.
func (h *Handler) signinRoute(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.onDashHost(r) {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			if !web.SameOrigin(r) {
				obs.Logger(r.Context()).Warn("dashboard sign-in refused: cross-site post", "origin", r.Header.Get("Origin"), "path", r.URL.Path)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
		}
		next(w, r)
	}
}

func (h *Handler) root(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handler) signinForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.person(r); ok {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	h.render(w, r, http.StatusOK, "signin.html", map[string]any{})
}

func (h *Handler) signinSubmit(w http.ResponseWriter, r *http.Request) {
	log := obs.Logger(r.Context())
	email, ok := authn.NormalizeEmail(r.PostFormValue("email"))
	if !ok {
		h.render(w, r, http.StatusBadRequest, "signin.html", map[string]any{"Error": "Enter a valid email address."})
		return
	}
	now := h.now()
	// Every outcome below renders the same page, so the answer never says who has an account.
	if !h.byIP.Allow(web.ClientIP(r), maxPostsPerIP, rateWindow, now) || !h.byEmail.Allow(email, maxEmailsPerAddress, rateWindow, now) {
		log.Warn("dashboard sign-in refused: rate limited")
		h.render(w, r, http.StatusOK, "signin_sent.html", map[string]any{})
		return
	}
	raw, ok, err := h.Auth.StartDashboardLogin(r.Context(), email, now)
	if err != nil {
		log.Error("dashboard sign-in start failed", "err", err)
	} else if ok && h.Mail != nil {
		h.sendLink(r.Context(), email, raw)
	}
	h.render(w, r, http.StatusOK, "signin_sent.html", map[string]any{})
}

// sendLink emails the link in the background, so a slow mail provider doesn't show in the response time
// of people who have an account.
func (h *Handler) sendLink(ctx context.Context, email, raw string) {
	link := strings.TrimRight(h.DashURL, "/") + "/dashboard/login/verify?" + url.Values{"t": {raw}}.Encode()
	m := mail.DashboardLoginMessage(email, link, h.SiteURL)
	log := obs.Logger(ctx)
	ctx = context.WithoutCancel(ctx)
	h.sending.Add(1)
	go func() {
		defer h.sending.Done()
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := h.Mail.Send(ctx, m); err != nil {
			log.Error("dashboard sign-in email failed", "err", err)
		}
	}()
}

// verifyPage shows a button and never touches the token: mail scanners open links before people do.
func (h *Handler) verifyPage(w http.ResponseWriter, r *http.Request) {
	t := r.URL.Query().Get("t")
	if t == "" {
		h.render(w, r, http.StatusBadRequest, "signin_error.html", map[string]any{})
		return
	}
	h.render(w, r, http.StatusOK, "signin_verify.html", map[string]any{"T": t})
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	raw := r.PostFormValue("t")
	if raw == "" {
		h.render(w, r, http.StatusBadRequest, "signin_error.html", map[string]any{})
		return
	}
	sess, err := h.Auth.FinishDashboardLogin(r.Context(), raw, h.now())
	if err != nil {
		if err != store.ErrInvalidToken {
			obs.Logger(r.Context()).Error("dashboard sign-in finish failed", "err", err)
		}
		h.render(w, r, http.StatusBadRequest, "signin_error.html", map[string]any{})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: sess.Token, Path: "/", MaxAge: int(sess.ExpiresAt.Sub(h.now()).Seconds()),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handler) signOut(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		if err := h.Auth.RevokeDashboardSession(r.Context(), c.Value); err != nil {
			obs.Logger(r.Context()).Error("dashboard sign-out failed", "err", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
}

// emailFormURL is app sign-in with the email form forced: sso=0 stops it hopping back here (ADR-039).
func (h *Handler) emailFormURL(slug, ret string) string {
	return strings.TrimRight(h.AuthURL, "/") + "/login?" + url.Values{"app": {slug}, "return": {ret}, "sso": {"0"}}.Encode()
}

// sso lets a signed-in owner or admin of an app's account into the app without the email step. The visitor
// arrives from the login page on the auth host, which cannot see this host's cookie. Anyone else is sent on to
// the email form, or told why they were not let in (ADR-039).
func (h *Handler) sso(w http.ResponseWriter, r *http.Request) {
	slug, ret := authn.CleanSlug(r.URL.Query().Get("app")), authn.SafeReturn(r.URL.Query().Get("return"))
	if slug == "" {
		h.notFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	p, ok := h.person(r)
	if !ok || p.Founder || p.UserID == 0 { // the founder key is not a user: it has no account roles to check
		http.Redirect(w, r, h.emailFormURL(slug, ret), http.StatusSeeOther)
		return
	}
	if !h.bySSO.Allow(strconv.FormatInt(p.UserID, 10), maxSSOPerUser, rateWindow, h.now()) {
		obs.Logger(r.Context()).Warn("dashboard sso refused: rate limited", "user", p.UserID)
		http.Redirect(w, r, h.emailFormURL(slug, ret), http.StatusSeeOther)
		return
	}
	code, err := h.Auth.IssueSSOCode(r.Context(), p.UserID, slug, h.now())
	switch {
	case errors.Is(err, store.ErrNoSSO):
		h.render(w, r, http.StatusForbidden, "sso_denied.html", map[string]any{"Email": p.Email, "LoginURL": h.emailFormURL(slug, ret)})
	case err != nil:
		h.fail(w, r, err)
	default:
		dest := url.URL{Scheme: h.AppsScheme, Host: slug + "." + h.AppsDomain, Path: "/__foyr/callback",
			RawQuery: url.Values{"code": {code}, "return": {ret}}.Encode()}
		http.Redirect(w, r, dest.String(), http.StatusSeeOther)
	}
}
