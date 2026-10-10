package main

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// newInternalServer serves endpoints only authproxy may call, on the private listener (ADR-030).
func newInternalServer(st *store.Store, token string, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /internal/routes", requireToken(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routes, err := st.Routes(r.Context())
		if err != nil {
			obs.Logger(r.Context()).Error("list routes failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if routes == nil {
			routes = []store.Route{}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"routes": routes})
	})))
	return obs.RequestID(log, mux)
}

// requireToken accepts only "Authorization: Bearer <token>", compared in constant time.
func requireToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || !ok || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
