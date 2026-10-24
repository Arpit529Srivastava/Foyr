package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name, xff, remote, want string
	}{
		{"rightmost entry is the load balancer's", "6.6.6.6, 203.0.113.9", "10.0.0.1:5555", "203.0.113.9"},
		{"spoofed left entries are ignored", "1.2.3.4, 5.6.7.8, 203.0.113.9", "10.0.0.1:5555", "203.0.113.9"},
		{"junk header falls back to the socket", "not-an-ip", "10.0.0.1:5555", "10.0.0.1"},
		{"no header uses the socket", "", "192.0.2.7:1234", "192.0.2.7"},
		{"no port", "", "192.0.2.7", "192.0.2.7"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := ClientIP(r); got != c.want {
			t.Errorf("%s: ClientIP = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		origin, host string
		want         bool
	}{
		{"https://auth.example.com", "auth.example.com", true},
		{"https://AUTH.example.com", "auth.example.com", true},
		{"http://localhost:8080", "localhost:8080", true},
		{"https://evil.com", "auth.example.com", false},
		{"https://auth.example.com.evil.com", "auth.example.com", false},
		{"null", "auth.example.com", false},
		{"", "auth.example.com", false},
		{"ftp://auth.example.com", "auth.example.com", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := SameOrigin(r); got != c.want {
			t.Errorf("Origin %q Host %q: SameOrigin = %v, want %v", c.origin, c.host, got, c.want)
		}
	}
}
