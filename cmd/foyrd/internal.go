package main

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
	"github.com/Arpit529Srivastava/Foyr/internal/obs"
	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// newInternalServer serves endpoints only authproxy may call, on the private listener (ADR-030).
// The session exchange and logout exist only when login is on, that is when key is set (ADR-035).
func newInternalServer(st *store.Store, token string, log *slog.Logger, key ed25519.PrivateKey) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /internal/access", requireToken(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snap, err := st.Access(r.Context())
		if err != nil {
			obs.Logger(r.Context()).Error("read access failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, snap)
	})))
	if len(key) > 0 {
		mux.Handle("POST /internal/auth/exchange", requireToken(token, exchange(st, key)))
		mux.Handle("POST /internal/auth/logout", requireToken(token, logout(st)))
	}
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
		writeJSON(w, http.StatusOK, map[string]any{"routes": routes})
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decode reads a small JSON body into v; false means a 400 was sent.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return false
	}
	return true
}

// exchange swaps a one-time code for a signed session token that authproxy puts in its cookie.
func exchange(st *store.Store, key ed25519.PrivateKey) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Code, Slug string }
		if !decode(w, r, &in) {
			return
		}
		sess, err := st.ExchangeCode(r.Context(), in.Code, in.Slug, time.Now())
		if errors.Is(err, store.ErrInvalidToken) {
			http.Error(w, "invalid or expired code", http.StatusBadRequest)
			return
		}
		if err != nil {
			obs.Logger(r.Context()).Error("exchange code failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		token := authn.Sign(key, authn.Claims{SID: sess.ID, UserID: sess.UserID, Email: sess.Email, Slug: sess.Slug, Exp: sess.ExpiresAt.Unix()})
		writeJSON(w, http.StatusOK, map[string]any{"token": token, "expires_at": sess.ExpiresAt.UTC().Format(time.RFC3339)})
	}
}

// logout revokes one session; authproxy learns of it from the next access snapshot.
func logout(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct{ SID string }
		if !decode(w, r, &in) {
			return
		}
		if err := st.RevokeSession(r.Context(), in.SID); err != nil {
			obs.Logger(r.Context()).Error("revoke session failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
