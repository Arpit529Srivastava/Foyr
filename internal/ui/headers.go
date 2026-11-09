package ui

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

// SecureHeaders sets the headers every foyrd HTML page sends and returns the nonce for its one script.
func SecureHeaders(w http.ResponseWriter) string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	nonce := base64.RawURLEncoding.EncodeToString(b)
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"'; style-src 'unsafe-inline'; img-src 'self' data:; font-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "same-origin") // "no-referrer" makes browsers send Origin: null on our own forms
	h.Set("X-Robots-Tag", "noindex, nofollow")
	return nonce
}
