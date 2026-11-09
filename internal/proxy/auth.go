package proxy

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/web"
)

// SessionCookie is host-only and HTTPS-only by the __Host- prefix rules, so every app host has its own.
const SessionCookie = "__Host-foyr_session"

// freshSession is how long after sign-in a missing membership is put down to a stale snapshot.
const freshSession = 20 * time.Second

const (
	callbackPath = "/__foyr/callback"
	logoutPath   = "/__foyr/logout"
)

// Auth is the login check authproxy runs in front of every app (ADR-035). A nil Handler.Auth turns it off.
type Auth struct {
	PublicKey ed25519.PublicKey
	AuthURL   string // https://auth.<PLATFORM_DOMAIN>
	Exchanger Exchanger
	Now       func() time.Time

	AssertionKey *ecdsa.PrivateKey // signs the identity assertion sent to apps
}

func (a *Auth) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// identity is what the app is told about the visitor.
type identity struct {
	userID   int64
	publicID string // usr_ id, the only user id apps see
	email    string
	role     string
}

// session returns the visitor's claims when the cookie is a genuine, unexpired token for this app.
func (a *Auth) session(r *http.Request, slug string) (authn.Claims, bool) {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return authn.Claims{}, false
	}
	claims, err := authn.Verify(a.PublicKey, c.Value, a.now())
	if err != nil || claims.Slug != slug || claims.PublicID == "" { // no pid: issued before the contract, sign in again
		return authn.Claims{}, false
	}
	return claims, true
}

// assertion signs a short-lived statement of who the visitor is for this app.
func (a *Auth) assertion(slug string, id *identity) (string, error) {
	if a.AssertionKey == nil {
		return "", errors.New("no assertion key")
	}
	return authn.SignAssertion(a.AssertionKey, authn.NewAssertion(a.AuthURL, slug, id.publicID, id.email, id.role, a.now()))
}

func (a *Auth) loginURL(slug, ret string) string {
	return a.AuthURL + "/login?" + url.Values{"app": {slug}, "return": {ret}}.Encode()
}

// authorize decides what happens to a request for an app. It returns the visitor's identity (nil when
// unknown) to pass on, or done=true when it has already answered.
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request, slug string) (id *identity, done bool) {
	a := h.Auth
	switch {
	case r.URL.Path == callbackPath && r.Method == http.MethodGet:
		h.callback(w, r, slug)
		return nil, true
	case r.URL.Path == logoutPath && r.Method == http.MethodPost:
		h.logout(w, r)
		return nil, true
	}
	if !h.Access.Ready() {
		page(w, http.StatusServiceUnavailable, "Starting up", "Foyr is loading access rules. Try again in a few seconds.")
		return nil, true
	}
	app, known := h.Access.App(slug)
	if !known {
		app = authn.AppAccess{Slug: slug, Mode: "private"} // not in the snapshot yet: closed until it is
	}
	claims, signedIn := a.session(r, slug)
	if signedIn && h.Access.Revoked(claims.SID) {
		signedIn = false
	}
	role, member := app.Members[claims.UserID]
	if signedIn && (member || app.Mode == "public") {
		if !member {
			role = "viewer"
		}
		id = &identity{userID: claims.UserID, publicID: claims.PublicID, email: claims.Email, role: role}
	}
	switch {
	case app.Mode == "public":
		return id, false
	case id != nil:
		return id, false
	case signedIn && a.now().Sub(time.Unix(claims.Iat, 0)) < freshSession:
		// Just signed in: foyrd knows the new member, our snapshot catches up within a poll or two.
		w.Header().Set("Retry-After", "2")
		pageWith(w, http.StatusServiceUnavailable, "Finishing sign-in", `<meta http-equiv="refresh" content="2">`,
			"<p>One moment. This page reloads by itself.</p>")
		return nil, true
	case signedIn: // genuine session, but not a member of this app
		pageHTML(w, http.StatusForbidden, "No access",
			"<p>"+html.EscapeString("You don't have access to "+slug+". Ask its owner to invite "+claims.Email+".")+"</p>"+
				`<form method="post" action="`+logoutPath+`"><button>Sign out</button></form>`)
		return nil, true
	}
	h.loginRequired(w, r, slug)
	return nil, true
}

// loginRequired sends browsers to sign in and tells everything else with a 401.
func (h *Handler) loginRequired(w http.ResponseWriter, r *http.Request, slug string) {
	login := h.Auth.loginURL(slug, r.URL.RequestURI())
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, login, http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "login required", "login_url": login})
}

// callback swaps the one-time code for a session cookie on this app's host.
func (h *Handler) callback(w http.ResponseWriter, r *http.Request, slug string) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	code := r.URL.Query().Get("code")
	if code == "" {
		h.expired(w, slug)
		return
	}
	token, exp, err := h.Auth.Exchanger.Exchange(r.Context(), code, slug)
	if err != nil {
		obs.Logger(r.Context()).Warn("login code exchange failed", "err", err)
		h.expired(w, slug)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: token, Path: "/", Expires: exp,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, authn.SafeReturn(r.URL.Query().Get("return")), http.StatusSeeOther)
}

func (h *Handler) expired(w http.ResponseWriter, slug string) {
	link := h.Auth.loginURL(slug, "/")
	pageHTML(w, http.StatusBadRequest, "This sign-in link has expired",
		`<p>Sign-in links work once, for a short time.</p><p><a href="`+html.EscapeString(link)+`">Sign in again</a></p>`)
}

// logout ends the session at foyrd and clears the cookie. Same-origin only, so other sites can't sign people out.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if !web.SameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(SessionCookie); err == nil {
		if claims, err := authn.Verify(h.Auth.PublicKey, c.Value, h.Auth.now()); err == nil {
			if err := h.Auth.Exchanger.Logout(r.Context(), claims.SID); err != nil {
				obs.Logger(r.Context()).Warn("logout at foyrd failed", "err", err)
			}
		}
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// dropCookie removes one cookie from every Cookie header and leaves the others byte for byte.
func dropCookie(h http.Header, name string) {
	vals := h.Values("Cookie")
	h.Del("Cookie")
	for _, v := range vals {
		var keep []string
		for _, part := range strings.Split(v, ";") {
			part = strings.TrimSpace(part)
			if part != "" && !strings.HasPrefix(part, name+"=") {
				keep = append(keep, part)
			}
		}
		if len(keep) > 0 {
			h.Add("Cookie", strings.Join(keep, "; "))
		}
	}
}
