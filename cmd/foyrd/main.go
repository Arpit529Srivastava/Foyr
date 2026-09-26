// Command foyrd is a Foyr service (placeholder until Phase 0 features land).
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Arpit529Srivastava/Foyr/internal/health"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health.Handler)
	log.Printf("foyrd listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
