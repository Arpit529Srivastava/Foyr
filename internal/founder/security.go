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

// Lockout limits (ADR-033): per address, and across all addresses against spread-out guessing.
const (
	MaxFailsPerIP = 5
	MaxFailsTotal = 30
	LockoutWindow = 15 * time.Minute
	maxTracked    = 10000 // addresses remembered at once, so a flood can't exhaust memory
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

// limiter counts failed sign-ins per address and in total over a sliding window.
type limiter struct {
	mu    sync.Mutex
	perIP map[string][]time.Time
	all   []time.Time
}

func recent(ts []time.Time, now time.Time) []time.Time {
	cut := now.Add(-LockoutWindow)
	i := 0
	for i < len(ts) && !ts[i].After(cut) {
		i++
	}
	return ts[i:]
}

// blocked reports whether ip may not try now, and for how long.
func (l *limiter) blocked(ip string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.all = recent(l.all, now)
	mine := recent(l.perIP[ip], now)
	if len(mine) == 0 {
		delete(l.perIP, ip)
	} else {
		l.perIP[ip] = mine
	}
	switch {
	case len(mine) >= MaxFailsPerIP:
		return true, mine[0].Add(LockoutWindow).Sub(now)
	case len(l.all) >= MaxFailsTotal:
		return true, l.all[0].Add(LockoutWindow).Sub(now)
	}
	return false, 0
}

func (l *limiter) fail(ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.perIP == nil {
		l.perIP = map[string][]time.Time{}
	}
	if _, ok := l.perIP[ip]; !ok && len(l.perIP) >= maxTracked {
		for k := range l.perIP { // drop an arbitrary old entry; the total limit still applies
			delete(l.perIP, k)
			break
		}
	}
	l.perIP[ip] = append(l.perIP[ip], now)
	l.all = append(l.all, now)
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
