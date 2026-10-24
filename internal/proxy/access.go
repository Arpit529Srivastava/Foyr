package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
)

type accessState struct {
	apps    map[string]authn.AppAccess
	revoked map[string]struct{}
}

// Access holds who may open each app, replaced whole by the poller; reads are lock-free.
type Access struct {
	state atomic.Pointer[accessState]
}

// Set replaces the snapshot.
func (a *Access) Set(s authn.AccessSnapshot) {
	st := &accessState{apps: make(map[string]authn.AppAccess, len(s.Apps)), revoked: make(map[string]struct{}, len(s.Revoked))}
	for _, app := range s.Apps {
		st.apps[app.Slug] = app
	}
	for _, id := range s.Revoked {
		st.revoked[id] = struct{}{}
	}
	a.state.Store(st)
}

// App returns an app's access rules, or false when the snapshot does not know the slug.
func (a *Access) App(slug string) (authn.AppAccess, bool) {
	st := a.state.Load()
	if st == nil {
		return authn.AppAccess{}, false
	}
	app, ok := st.apps[slug]
	return app, ok
}

// Revoked reports whether a session was ended before it expired.
func (a *Access) Revoked(sid string) bool {
	st := a.state.Load()
	if st == nil {
		return false
	}
	_, ok := st.revoked[sid]
	return ok
}

// Ready reports whether a snapshot has been loaded at least once.
func (a *Access) Ready() bool { return a.state.Load() != nil }

// AccessPoller refreshes Access from foyrd's /internal/access. On errors it keeps the last good snapshot,
// so people stay signed in while foyrd is down.
type AccessPoller struct {
	URL    string // e.g. http://foyrd.platform.foyr.internal:9090/internal/access
	Token  string
	Every  time.Duration
	HTTP   *http.Client
	Access *Access
	Log    *slog.Logger
}

// Run polls until ctx ends.
func (p *AccessPoller) Run(ctx context.Context) {
	every := p.Every
	if every <= 0 {
		every = 3 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	last := ""
	for {
		if snap, err := p.Fetch(ctx); err != nil {
			p.Log.Warn("access refresh failed; keeping the last good snapshot", "err", err)
		} else {
			p.Access.Set(snap)
			if sig := fmt.Sprintf("apps=%d revoked=%d", len(snap.Apps), len(snap.Revoked)); sig != last {
				p.Log.Info("access updated", "apps", len(snap.Apps), "revoked_sessions", len(snap.Revoked))
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

// Fetch loads the snapshot once.
func (p *AccessPoller) Fetch(ctx context.Context) (authn.AccessSnapshot, error) {
	var snap authn.AccessSnapshot
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return snap, err
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	client := p.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return snap, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return snap, fmt.Errorf("access: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 16<<20)).Decode(&snap); err != nil {
		return snap, fmt.Errorf("access: %w", err)
	}
	return snap, nil
}

// Exchanger talks to foyrd for the two things authproxy cannot do itself: turn a one-time code into a
// session token, and end a session.
type Exchanger interface {
	Exchange(ctx context.Context, code, slug string) (token string, exp time.Time, err error)
	Logout(ctx context.Context, sid string) error
}

// HTTPExchanger calls foyrd's internal listener.
type HTTPExchanger struct {
	BaseURL string // e.g. http://foyrd.platform.foyr.internal:9090
	Token   string
	HTTP    *http.Client
}

func (x *HTTPExchanger) post(ctx context.Context, path string, body any) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+x.Token)
	req.Header.Set("Content-Type", "application/json")
	client := x.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = cancelOnClose{resp.Body, cancel}
	return resp, nil
}

// cancelOnClose keeps the request context alive until the body has been read.
type cancelOnClose struct {
	rc interface {
		Read([]byte) (int, error)
		Close() error
	}
	cancel context.CancelFunc
}

func (c cancelOnClose) Read(p []byte) (int, error) { return c.rc.Read(p) }
func (c cancelOnClose) Close() error               { defer c.cancel(); return c.rc.Close() }

// Exchange spends a one-time code for a session token for slug.
func (x *HTTPExchanger) Exchange(ctx context.Context, code, slug string) (string, time.Time, error) {
	resp, err := x.post(ctx, "/internal/auth/exchange", map[string]string{"code": code, "slug": slug})
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("exchange: status %d", resp.StatusCode)
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&out); err != nil || out.Token == "" {
		return "", time.Time{}, fmt.Errorf("exchange: bad answer")
	}
	return out.Token, out.ExpiresAt, nil
}

// Logout ends a session.
func (x *HTTPExchanger) Logout(ctx context.Context, sid string) error {
	resp, err := x.post(ctx, "/internal/auth/logout", map[string]string{"sid": sid})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("logout: status %d", resp.StatusCode)
	}
	return nil
}
