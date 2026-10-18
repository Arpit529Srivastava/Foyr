package founder

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// clock is a settable time source shared by a handler and its test.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func secureHandler(c *clock) (*Handler, http.Handler) {
	h := &Handler{Data: fake{status: "live"}, Key: key, Now: c.now, FailDelay: -1}
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

func login(mux http.Handler, k, ip, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/founder/login", strings.NewReader(url.Values{"key": {k}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	r.RemoteAddr = "10.0.1.5:4242" // the load balancer
	if ip != "" {
		r.Header.Set("X-Forwarded-For", ip)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestSessionExpiresOnTheServer(t *testing.T) {
	c := &clock{t0}
	_, mux := secureHandler(c)
	w := login(mux, key, "1.2.3.4", "http://example.com")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("login = %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	get := func() int {
		r := httptest.NewRequest("GET", "/founder/activity", nil)
		r.AddCookie(cookie)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		return rec.Code
	}
	c.t = t0.Add(SessionTTL - time.Minute)
	if got := get(); got != 200 {
		t.Fatalf("before expiry = %d", got)
	}
	c.t = t0.Add(SessionTTL + time.Second)
	if got := get(); got != http.StatusSeeOther {
		t.Fatalf("after expiry = %d, want a redirect to sign in", got)
	}
}

func TestSessionsAreUniqueAndTamperProof(t *testing.T) {
	h := &Handler{Key: key}
	a, b := h.newSession(t0), h.newSession(t0)
	if a == b {
		t.Fatal("two logins got the same session")
	}
	if !h.validSession(a, t0) {
		t.Fatal("fresh session rejected")
	}
	parts := strings.Split(a, ".")
	parts[1] = "9999999999" // push the expiry far out
	for name, v := range map[string]string{
		"changed expiry": strings.Join(parts, "."),
		"changed mac":    a[:len(a)-1] + map[bool]string{true: "0", false: "1"}[a[len(a)-1] != '0'],
		"old v1 cookie":  "6c1f0a0e9b0f6bb5",
		"empty":          "",
		"junk":           "v2.x.y.z",
	} {
		if h.validSession(v, t0) {
			t.Errorf("%s accepted", name)
		}
	}
	if (&Handler{Key: strings.Repeat("z", 32)}).validSession(a, t0) {
		t.Error("session from another key accepted")
	}
}

func TestLoginLocksOutOneAddressAfterFiveMisses(t *testing.T) {
	c := &clock{t0}
	_, mux := secureHandler(c)
	for i := range MaxFailsPerIP {
		if w := login(mux, "wrong-key-wrong-key-wrong-key-00", "1.2.3.4", "http://example.com"); w.Code != http.StatusUnauthorized {
			t.Fatalf("miss %d = %d", i+1, w.Code)
		}
	}
	w := login(mux, key, "1.2.3.4", "http://example.com")
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" || len(w.Result().Cookies()) != 0 {
		t.Fatalf("locked address with the right key = %d, cookies %v", w.Code, w.Result().Cookies())
	}
	if w := login(mux, key, "5.6.7.8", "http://example.com"); w.Code != http.StatusSeeOther {
		t.Fatalf("another address = %d, want to sign in", w.Code)
	}
	c.t = t0.Add(LockoutWindow + time.Second)
	if w := login(mux, key, "1.2.3.4", "http://example.com"); w.Code != http.StatusSeeOther {
		t.Fatalf("after the window = %d", w.Code)
	}
}

func TestSpreadAttackSlowsSignInsButCantLockTheFounderOut(t *testing.T) {
	c := &clock{t0}
	_, mux := secureHandler(c)
	for i := range MaxFailsTotal {
		login(mux, "wrong-key-wrong-key-wrong-key-00", "9.9.9."+strconv.Itoa(i), "http://example.com")
	}
	if w := login(mux, key, "5.6.7.8", "http://example.com"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("right after a spread attack = %d, want 429 (slow mode)", w.Code)
	}
	c.t = c.t.Add(SlowModeGap + time.Second)
	if w := login(mux, key, "5.6.7.8", "http://example.com"); w.Code != http.StatusSeeOther {
		t.Fatalf("founder after the slow-mode gap = %d, want to sign in", w.Code)
	}
}

// Parallel guesses from one address must not all slip past the check before any failure is counted.
func TestParallelGuessesCantBeatTheLockout(t *testing.T) {
	h := &Handler{Data: fake{status: "live"}, Key: key, Now: func() time.Time { return t0 }, FailDelay: 50 * time.Millisecond}
	mux := http.NewServeMux()
	h.Register(mux)
	var wg sync.WaitGroup
	var tried atomic.Int32
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if login(mux, "wrong-key-wrong-key-wrong-key-00", "1.2.3.4", "http://example.com").Code == http.StatusUnauthorized {
				tried.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := tried.Load(); n > MaxFailsPerIP {
		t.Fatalf("%d guesses were checked, want at most %d", n, MaxFailsPerIP)
	}
}

func TestClientAddressIsTheOneTheLoadBalancerAdded(t *testing.T) {
	c := &clock{t0}
	_, mux := secureHandler(c)
	// The attacker controls everything left of the load balancer's own entry, so changing it must not help.
	for i := range MaxFailsPerIP {
		login(mux, "wrong-key-wrong-key-wrong-key-00", "6.6.6."+string(rune('0'+i))+", 1.2.3.4", "http://example.com")
	}
	if w := login(mux, key, "7.7.7.7, 1.2.3.4", "http://example.com"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("spoofed left-hand addresses escaped the lockout: %d", w.Code)
	}
}

func TestPostsMustComeFromThePageItself(t *testing.T) {
	c := &clock{t0}
	_, mux := secureHandler(c)
	for name, origin := range map[string]string{"missing": "", "other site": "https://evil.example", "lookalike": "http://example.com.evil.test"} {
		if w := login(mux, key, "1.2.3.4", origin); w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
			t.Errorf("%s origin = %d", name, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/founder/logout", nil)
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-site logout = %d", w.Code)
	}
}

func TestWrongKeysAreSlowed(t *testing.T) {
	h := &Handler{Data: fake{status: "live"}, Key: key, FailDelay: 120 * time.Millisecond}
	mux := http.NewServeMux()
	h.Register(mux)
	start := time.Now()
	login(mux, "wrong-key-wrong-key-wrong-key-00", "1.2.3.4", "http://example.com")
	if time.Since(start) < 100*time.Millisecond {
		t.Fatal("a wrong key answered without the delay")
	}
}

func TestHSTSAndFraming(t *testing.T) {
	_, mux := secureHandler(&clock{t0})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/founder", nil))
	if !strings.Contains(w.Header().Get("Strict-Transport-Security"), "max-age=") || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("headers = %v", w.Header())
	}
}

func TestOnlyTheNoncedScriptMayRun(t *testing.T) {
	c := &clock{t0}
	_, mux := secureHandler(c)
	cookie := login(mux, key, "1.2.3.4", "http://example.com").Result().Cookies()[0]
	nonces := map[string]bool{}
	for range 2 {
		r := httptest.NewRequest("GET", "/founder", nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		csp := w.Header().Get("Content-Security-Policy")
		i := strings.Index(csp, "'nonce-")
		if i < 0 || strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "script-src 'unsafe-inline'") {
			t.Fatalf("csp = %q", csp)
		}
		n := csp[i+7 : i+7+strings.Index(csp[i+7:], "'")]
		if len(n) < 22 || !strings.Contains(w.Body.String(), `<script nonce="`+n+`">`) {
			t.Fatalf("nonce %q not on the page's script", n)
		}
		nonces[n] = true
	}
	if len(nonces) != 2 {
		t.Fatal("the nonce must change on every page")
	}
}

func TestSidebarRemembersBeingHidden(t *testing.T) {
	c := &clock{t0}
	_, mux := secureHandler(c)
	cookie := login(mux, key, "1.2.3.4", "http://example.com").Result().Cookies()[0]
	page := func(pref string) string {
		r := httptest.NewRequest("GET", "/founder", nil)
		r.AddCookie(cookie)
		if pref != "" {
			r.AddCookie(&http.Cookie{Name: SidebarCookie, Value: pref})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Body.String()
	}
	if b := page(""); !strings.Contains(b, `id="side-toggle"`) || strings.Contains(b, `id="side-toggle" checked`) {
		t.Error("sidebar should start open with a toggle")
	}
	if !strings.Contains(page("collapsed"), `id="side-toggle" checked`) {
		t.Error("collapsed preference not applied")
	}
	if strings.Contains(page(`"><script>`), "<script>\"") {
		t.Error("preference cookie must not be echoed")
	}
}
