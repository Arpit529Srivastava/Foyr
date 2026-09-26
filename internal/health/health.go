// Package health serves the liveness endpoint used by load balancers.
package health

import "net/http"

// Handler answers 200 "ok" for GET /healthz.
func Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}
