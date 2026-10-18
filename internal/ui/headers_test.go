package ui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecureHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	n1, n2 := SecureHeaders(w), SecureHeaders(httptest.NewRecorder())
	if len(n1) < 22 || n1 == n2 {
		t.Fatalf("nonces %q %q", n1, n2)
	}
	h := w.Header()
	if csp := h.Get("Content-Security-Policy"); !strings.HasPrefix(csp, "default-src 'none'") || !strings.Contains(csp, "'nonce-"+n1+"'") {
		t.Errorf("csp = %q", csp)
	}
	for k, v := range map[string]string{"X-Frame-Options": "DENY", "Cache-Control": "no-store", "Referrer-Policy": "same-origin"} {
		if h.Get(k) != v {
			t.Errorf("%s = %q", k, h.Get(k))
		}
	}
}
