package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

const testInternalToken = "internal-token-internal-token-123456"

func TestInternalRoutes(t *testing.T) {
	e := setup(t)
	h := newInternalServer(e.st, testInternalToken, obs.NewLogger(e.logs))
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
