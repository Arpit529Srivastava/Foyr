// Package web holds small HTTP helpers shared by foyrd's pages.
package web

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// ClientIP is the address the load balancer saw: the rightmost X-Forwarded-For entry, which it appends
// itself. Entries to the left are whatever the visitor sent and can't be trusted.
func ClientIP(r *http.Request) string {
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

// SameOrigin accepts a form post only when the browser says it came from this host.
func SameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" || o == "null" {
		return false
	}
	u, err := url.Parse(o)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && strings.EqualFold(u.Host, r.Host)
}
