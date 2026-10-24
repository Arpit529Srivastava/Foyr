package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

const testInternalToken = "internal-token-internal-token-123456"

func TestInternalRoutes(t *testing.T) {
	e := setup(t)
	h := newInternalServer(e.st, testInternalToken, obs.NewLogger(e.logs), nil)
	get := func(auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/internal/routes", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for _, auth := range []string{"", "Bearer wrong", testInternalToken, "Basic " + testInternalToken} {
		if rec := get(auth); rec.Code != http.StatusUnauthorized {
			t.Errorf("auth %q: status %d, want 401", auth, rec.Code)
		}
	}

	rec := get("Bearer " + testInternalToken)
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"routes\":[]}\n" {
		t.Fatalf("empty: %d %s", rec.Code, rec.Body)
	}

	e.post("push", "d-1", fixture(t, "push.json"))
	for _, to := range []string{store.StatusBuilding, store.StatusBuilt, store.StatusDeploying} {
		if _, err := e.st.SetDeploymentStatus(ctx, 1, store.StatusChange{To: to}); err != nil {
			t.Fatal(err)
		}
	}
	_ = e.st.SetRun(ctx, 1, "arn:td", "d-1")
	if _, err := e.st.MarkLive(ctx, 1, store.System); err != nil {
		t.Fatal(err)
	}
	var body struct{ Routes []store.Route }
	rec = get("Bearer " + testInternalToken)
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || len(body.Routes) != 1 ||
		body.Routes[0].Slug != "foyr-hello" || body.Routes[0].Service != "d-1" {
		t.Fatalf("routes = %+v err=%v", body.Routes, err)
	}
}

func TestInternalRoutesNotOnPublicServer(t *testing.T) {
	e := setup(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/routes", nil)
	req.Header.Set("Authorization", "Bearer "+testInternalToken)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public server answered /internal/routes with %d", rec.Code)
	}
}

// loginCode seeds the fixture app, invites email and returns a one-time code for it.
func loginCode(t *testing.T, e env, email string) (code, slug string) {
	t.Helper()
	e.post("push", "d-1", fixture(t, "push.json"))
	apps, err := e.st.ListApps(ctx)
	if err != nil || len(apps) != 1 {
		t.Fatalf("apps = %v, %v", apps, err)
	}
	if _, err := e.st.CreateInvite(ctx, apps[0].ID, email, "member", store.System); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := e.st.StartLogin(ctx, apps[0].Slug, email, time.Now())
	if err != nil || !ok {
		t.Fatalf("StartLogin = %v, %v", ok, err)
	}
	code, slug, err = e.st.FinishLogin(ctx, raw, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return code, slug
}

func internalCall(h http.Handler, method, path, auth, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestInternalAuthEndpointsNeedTheToken(t *testing.T) {
	e := setup(t)
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	h := newInternalServer(e.st, testInternalToken, obs.NewLogger(e.logs), priv)
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/internal/access", ""},
		{"POST", "/internal/auth/exchange", `{"code":"x","slug":"y"}`},
		{"POST", "/internal/auth/logout", `{"sid":"x"}`},
	} {
		for _, auth := range []string{"", "Bearer wrong"} {
			if rec := internalCall(h, c.method, c.path, auth, c.body); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with auth %q: %d, want 401", c.method, c.path, auth, rec.Code)
			}
		}
	}
}

func TestInternalAccess(t *testing.T) {
	e := setup(t)
	h := newInternalServer(e.st, testInternalToken, obs.NewLogger(e.logs), nil)
	rec := internalCall(h, "GET", "/internal/access", "Bearer "+testInternalToken, "")
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"apps\":[],\"revoked\":[]}\n" {
		t.Fatalf("empty: %d %s", rec.Code, rec.Body)
	}
	e.post("push", "d-1", fixture(t, "push.json"))
	var snap authn.AccessSnapshot
	rec = internalCall(h, "GET", "/internal/access", "Bearer "+testInternalToken, "")
	if err := json.NewDecoder(rec.Body).Decode(&snap); err != nil || len(snap.Apps) != 1 ||
		snap.Apps[0].Slug != "foyr-hello" || snap.Apps[0].Mode != "private" {
		t.Fatalf("snapshot = %+v, %v", snap, err)
	}
}

func TestInternalExchangeIssuesAVerifiableToken(t *testing.T) {
	e := setup(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	h := newInternalServer(e.st, testInternalToken, obs.NewLogger(e.logs), priv)
	code, slug := loginCode(t, e, "ana@example.com")
	auth := "Bearer " + testInternalToken

	if rec := internalCall(h, "POST", "/internal/auth/exchange", auth, `{"code":"nope","slug":"`+slug+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad code: %d, want 400", rec.Code)
	}
	if rec := internalCall(h, "POST", "/internal/auth/exchange", auth, `not json`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad body: %d, want 400", rec.Code)
	}
	rec := internalCall(h, "POST", "/internal/auth/exchange", auth, `{"code":"`+code+`","slug":"`+slug+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("exchange: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	claims, err := authn.Verify(pub, out.Token, time.Now())
	if err != nil || claims.Slug != slug || claims.Email != "ana@example.com" || claims.SID == "" || claims.UserID == 0 {
		t.Fatalf("claims = %+v, %v", claims, err)
	}
	if claims.Exp != out.ExpiresAt.Unix() || out.ExpiresAt.Before(time.Now().Add(6*24*time.Hour)) {
		t.Errorf("expiry %v does not match the 7-day session (exp %d)", out.ExpiresAt, claims.Exp)
	}
	if rec := internalCall(h, "POST", "/internal/auth/exchange", auth, `{"code":"`+code+`","slug":"`+slug+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("second exchange of the same code: %d, want 400", rec.Code)
	}

	// Logout revokes the session, and authproxy sees it in the next snapshot.
	if rec := internalCall(h, "POST", "/internal/auth/logout", auth, `{"sid":"`+claims.SID+`"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	snap, _ := e.st.Access(ctx)
	if len(snap.Revoked) != 1 || snap.Revoked[0] != claims.SID {
		t.Errorf("revoked = %v, want [%s]", snap.Revoked, claims.SID)
	}
}

func TestInternalAuthEndpointsAbsentWhenLoginIsOff(t *testing.T) {
	e := setup(t)
	h := newInternalServer(e.st, testInternalToken, obs.NewLogger(e.logs), nil)
	for _, p := range []string{"/internal/auth/exchange", "/internal/auth/logout"} {
		if rec := internalCall(h, "POST", p, "Bearer "+testInternalToken, "{}"); rec.Code != http.StatusNotFound {
			t.Errorf("%s with login off: %d, want 404", p, rec.Code)
		}
	}
}

func TestInternalBodiesAreCapped(t *testing.T) {
	e := setup(t)
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	h := newInternalServer(e.st, testInternalToken, obs.NewLogger(e.logs), priv)
	big := `{"code":"` + strings.Repeat("a", 10000) + `","slug":"x"}`
	if rec := internalCall(h, "POST", "/internal/auth/exchange", "Bearer "+testInternalToken, big); rec.Code != http.StatusBadRequest {
		t.Errorf("oversized body: %d, want 400", rec.Code)
	}
}
