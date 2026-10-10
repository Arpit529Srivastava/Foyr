// Package proxy is authproxy: it routes <slug>.<APPS_DOMAIN> to the app's live deployment (ADR-030).
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// Table maps app slugs to upstream host:port; reads are lock-free and safe while it is replaced.
type Table struct {
	routes atomic.Pointer[map[string]string]
}

// Lookup returns the upstream for slug.
func (t *Table) Lookup(slug string) (string, bool) {
	m := t.routes.Load()
	if m == nil {
		return "", false
	}
	up, ok := (*m)[slug]
	return up, ok
}

// Set replaces the whole table.
func (t *Table) Set(m map[string]string) { t.routes.Store(&m) }

// Len is the number of routes.
func (t *Table) Len() int {
	if m := t.routes.Load(); m != nil {
		return len(*m)
	}
	return 0
}

// Ready reports whether the table has been loaded at least once.
func (t *Table) Ready() bool { return t.routes.Load() != nil }

// Poller refreshes a Table from foyrd's /internal/routes. On errors it keeps the last good table,
// so apps keep serving while foyrd is down.
type Poller struct {
	URL       string // e.g. http://foyrd.platform.foyr.internal:9090/internal/routes
	Token     string
	Namespace string // Cloud Map domain, e.g. apps.foyr.internal
	Port      int    // app port, 3000
	Every     time.Duration
	HTTP      *http.Client
	Table     *Table
	Log       *slog.Logger
}

// Run polls until ctx ends.
func (p *Poller) Run(ctx context.Context) {
	every := p.Every
	if every <= 0 {
		every = 3 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	last := ""
	for {
		if m, err := p.Fetch(ctx); err != nil {
			p.Log.Warn("routes refresh failed; keeping the last good table", "err", err, "routes", p.Table.Len())
		} else {
			p.Table.Set(m)
			if sig := signature(m); sig != last {
				p.Log.Info("routes updated", "routes", len(m), "table", sig)
				last = sig
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Fetch loads the routes once.
func (p *Poller) Fetch(ctx context.Context) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("routes: status %d", resp.StatusCode)
	}
	var body struct {
		Routes []struct {
			Slug    string `json:"slug"`
			Service string `json:"service"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 8<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("routes: %w", err)
	}
	m := make(map[string]string, len(body.Routes))
	for _, r := range body.Routes {
		if r.Slug == "" || r.Service == "" {
			continue
		}
		m[r.Slug] = fmt.Sprintf("%s.%s:%d", r.Service, p.Namespace, p.Port)
	}
	return m, nil
}

// ParseStatic reads "slug=host:port,slug2=host:port" for local development.
func ParseStatic(s string) (map[string]string, error) {
	m := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		slug, up, ok := strings.Cut(part, "=")
		if !ok || slug == "" || up == "" {
			return nil, fmt.Errorf("static route %q must look like slug=host:port", part)
		}
		m[slug] = up
	}
	return m, nil
}

// signature is a short, stable description of the table for logs.
func signature(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+strings.SplitN(v, ".", 2)[0])
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
