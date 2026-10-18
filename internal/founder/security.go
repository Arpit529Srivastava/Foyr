package founder

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Lockout limits (ADR-033). Per address: a hard lockout. Across all addresses: a slow mode instead of a
// lockout, because with a 256-bit key a global lock would mostly let strangers lock the founder out.
const (
	MaxFailsPerIP = 5
	MaxFailsTotal = 30
	LockoutWindow = 15 * time.Minute
	SlowModeGap   = 10 * time.Second // in slow mode, one attempt per gap from anyone
	maxTracked    = 10000            // addresses remembered at once, so a flood can't exhaust memory
)

// newSession issues "v2.<expiry>.<random id>.<mac>"; the MAC binds all of it to the key.
func (h *Handler) newSession(now time.Time) string {
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	body := "v2." + strconv.FormatInt(now.Add(SessionTTL).Unix(), 10) + "." + hex.EncodeToString(id)
	return body + "." + h.mac(body)
}

// validSession checks the MAC in constant time, then the expiry.
func (h *Handler) validSession(v string, now time.Time) bool {
	i := strings.LastIndexByte(v, '.')
	if i < 0 || !strings.HasPrefix(v, "v2.") {
		return false
	}
	body, sig := v[:i], v[i+1:]
	if !hmac.Equal([]byte(sig), []byte(h.mac(body))) {
		return false
	}
	parts := strings.Split(body, ".")
	if len(parts) != 3 {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	return err == nil && now.Unix() < exp
}

func (h *Handler) mac(body string) string {
	m := hmac.New(sha256.New, []byte(h.Key))
	m.Write([]byte("foyr founder session|" + body))
	return hex.EncodeToString(m.Sum(nil))
}

// limiter counts sign-in attempts per address and in total over a sliding window. An attempt is counted
// when it starts and forgiven if the key was right, so parallel requests can't all pass one check.
type limiter struct {
	mu    sync.Mutex
	perIP map[string][]time.Time
	all   []time.Time
	last  time.Time // last attempt allowed, for slow mode
}

func recent(ts []time.Time, now time.Time) []time.Time {
	cut := now.Add(-LockoutWindow)
	i := 0
	for i < len(ts) && !ts[i].After(cut) {
		i++
	}
	return ts[i:]
}

// begin reserves an attempt for ip, or says how long to wait.
func (l *limiter) begin(ip string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.perIP == nil {
		l.perIP = map[string][]time.Time{}
	}
	l.all = recent(l.all, now)
	mine := recent(l.perIP[ip], now)
	switch {
	case len(mine) >= MaxFailsPerIP:
		l.perIP[ip] = mine
		return false, mine[0].Add(LockoutWindow).Sub(now)
	case len(l.all) >= MaxFailsTotal && now.Sub(l.last) < SlowModeGap:
		return false, SlowModeGap - now.Sub(l.last)
	}
	if _, ok := l.perIP[ip]; !ok && len(l.perIP) >= maxTracked {
		for k := range l.perIP { // drop an arbitrary entry; the total still counts it
			delete(l.perIP, k)
			break
		}
	}
	l.perIP[ip] = append(mine, now)
	l.all = append(l.all, now)
	l.last = now
	return true, 0
}

// forgive removes the attempt begun at t, after the right key.
func (l *limiter) forgive(ip string, t time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.perIP[ip] = drop(l.perIP[ip], t)
	if len(l.perIP[ip]) == 0 {
		delete(l.perIP, ip)
	}
	l.all = drop(l.all, t)
}

func drop(ts []time.Time, t time.Time) []time.Time {
	for i := len(ts) - 1; i >= 0; i-- {
		if ts[i].Equal(t) {
			return append(ts[:i], ts[i+1:]...)
		}
	}
	return ts
}

// clientIP is the address the load balancer saw: the rightmost X-Forwarded-For entry, which it appends
// itself. Entries to the left are whatever the visitor sent and can't be trusted.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(ip) != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// sameOrigin accepts a form post only when the browser says it came from this host.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" || o == "null" {
		return false
	}
	u, err := url.Parse(o)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && strings.EqualFold(u.Host, r.Host)
}

// newNonce returns a fresh value for the page's one allowed script.
func newNonce() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
