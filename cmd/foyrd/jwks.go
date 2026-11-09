package main

import (
	"crypto/ecdsa"
	"net/http"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
)

// jwksHandler serves the identity key set; the body is built once.
func jwksHandler(keys []*ecdsa.PublicKey) http.Handler {
	body := authn.JWKS(keys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "application/json")
		h.Set("Cache-Control", "public, max-age=300")
		h.Set("Access-Control-Allow-Origin", "*")
		w.Write(body)
	})
}
