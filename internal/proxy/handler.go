package proxy

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/slug"
)

// HealthPath is answered by authproxy itself on every host, for the load balancer.
const HealthPath = "/_foyr/healthz"

// Handler routes requests by hostname to the live deployment of each app.
type Handler struct {
	Domain    string // APPS_DOMAIN, e.g. apps.foyr.online
	Table     *Table
	Access    *Access // who may open each app; used when Auth is set
	Auth      *Auth   // nil turns login checks off
	Transport http.RoundTripper
	proxy     *httputil.ReverseProxy
}

// NewHandler builds the router.
func NewHandler(domain string, table *Table) *Handler {
	h := &Handler{Domain: strings.ToLower(strings.TrimSuffix(domain, ".")), Table: table, Transport: NewTransport()}
	h.proxy = &httputil.ReverseProxy{
		Rewrite: h.rewrite,
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return h.Transport.RoundTrip(r)
		}),
		ErrorHandler:  h.upstreamError,
		FlushInterval: -1, // stream responses (server-sent events, long polls)
	}
	return h
}

// NewTransport dials apps with timeouts so a stuck app can't hold connections forever.
func NewTransport() *http.Transport {
	return &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

type ctxKey struct{}

type route struct {
	slug, upstream string
	id             *identity // nil: the visitor is unknown
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == HealthPath {
		if !h.Table.Ready() || (h.Auth != nil && !h.Access.Ready()) {
			http.Error(w, "routes or access not loaded", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	s, ok := h.slugFor(r.Host)
	if !ok {
		page(w, http.StatusNotFound, "No app at this address", "Check the link, or ask the person who sent it.")
		return
	}
	up, ok := h.Table.Lookup(s)
	if !ok {
		page(w, http.StatusNotFound, "No app at this address",
			fmt.Sprintf("There is no running app called %q on Foyr. If it's yours, push to deploy it.", s))
		return
	}
	log := obs.Logger(r.Context()).With("app", s, "upstream", up)
	ctx := obs.WithLogger(r.Context(), log)
	var id *identity
	if h.Auth != nil {
		var done bool
		if id, done = h.authorize(w, r, s); done {
			return
		}
	}
	h.proxy.ServeHTTP(w, r.WithContext(withRoute(ctx, route{slug: s, upstream: up, id: id})))
}

// slugFor extracts the app slug from host, or false if host isn't <slug>.<Domain>.
func (h *Handler) slugFor(host string) (string, bool) {
	host = strings.ToLower(host)
	if hh, _, err := net.SplitHostPort(host); err == nil {
		host = hh
	}
	host = strings.TrimSuffix(host, ".")
	s, ok := strings.CutSuffix(host, "."+h.Domain)
	if !ok || strings.Contains(s, ".") || !slug.Valid(s) {
		return "", false
	}
	return s, true
}

// rewrite sends the request to the app with trusted headers only.
func (h *Handler) rewrite(pr *httputil.ProxyRequest) {
	rt := routeFrom(pr.In.Context())
	pr.Out.URL.Scheme = "http"
	pr.Out.URL.Host = rt.upstream
	pr.Out.Host = pr.In.Host

	// Visitors can't send identity headers; only authproxy sets them (ADR-006, Phase 1).
	for name := range pr.Out.Header {
		if strings.HasPrefix(strings.ToLower(name), "x-foyr-") {
			pr.Out.Header.Del(name)
		}
	}
	if rt.id != nil {
		pr.Out.Header.Set("X-Foyr-User-Id", strconv.FormatInt(rt.id.userID, 10))
		pr.Out.Header.Set("X-Foyr-Email", rt.id.email)
		pr.Out.Header.Set("X-Foyr-Role", rt.id.role)
	}
	// The session token is for authproxy; the app never sees it.
	dropCookie(pr.Out.Header, SessionCookie)
	// Only the load balancer can reach authproxy, so its forwarding headers are kept.
	if xff := pr.In.Header.Get("X-Forwarded-For"); xff != "" {
		pr.Out.Header.Set("X-Forwarded-For", xff)
	}
	pr.SetXForwarded()
	if proto := pr.In.Header.Get("X-Forwarded-Proto"); proto == "https" || proto == "http" {
		pr.Out.Header.Set("X-Forwarded-Proto", proto)
	}
	if id := obs.RequestIDFrom(pr.In.Context()); id != "" {
		pr.Out.Header.Set("X-Request-Id", id)
	}
}

// upstreamError answers when the app can't be reached or doesn't answer in time.
func (h *Handler) upstreamError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, r.Context().Err()) && r.Context().Err() != nil {
		return // the visitor went away
	}
	obs.Logger(r.Context()).Warn("app unreachable", "err", err)
	page(w, http.StatusBadGateway, "This app isn't responding",
		"It may be starting up or have crashed. Try again in a moment. If it's your app, check its deploy and logs in Foyr.")
}

func withRoute(ctx context.Context, rt route) context.Context {
	return context.WithValue(ctx, ctxKey{}, rt)
}

func routeFrom(ctx context.Context) route {
	rt, _ := ctx.Value(ctxKey{}).(route)
	return rt
}

// page writes a small Foyr-branded error page.
func page(w http.ResponseWriter, status int, title, body string) {
	pageHTML(w, status, title, "<p>"+html.EscapeString(body)+"</p>")
}

// pageHTML is page with body as safe HTML blocks (paragraphs, forms); callers escape whatever they interpolate.
func pageHTML(w http.ResponseWriter, status int, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>%[1]s · Foyr</title><style>
body{margin:0;min-height:100vh;display:grid;place-items:center;background:#0b0b0c;color:#ededed;font:16px/1.5 system-ui,-apple-system,sans-serif}
main{max-width:30rem;padding:2rem}p.k{color:#8a8a8f;font:12px ui-monospace,monospace;letter-spacing:.12em;text-transform:uppercase}
h1{font-size:1.75rem;margin:.25rem 0 .75rem}p{color:#b4b4b8}a{color:#ffd27a}
button{font:inherit;cursor:pointer;border-radius:999px;border:1px solid #484848;background:transparent;color:#ededed;padding:.5rem 1rem}
</style></head><body><main><p class="k">Foyr · %[3]d</p><h1>%[1]s</h1>%[2]s</main></body></html>`,
		html.EscapeString(title), body, status)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
