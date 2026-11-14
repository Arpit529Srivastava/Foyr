package authn

import (
	"strings"
	"testing"
)

func TestCleanSlug(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"crm", "crm"},
		{"foyr-hello", "foyr-hello"},
		{"a1", "a1"},
		{"", ""},
		{"A..B", ""},
		{"Crm", ""},
		{"evil.com/x", ""},
		{"-a", ""},
		{"a-", ""},
		{"a", ""},
		{strings.Repeat("a", 41), ""},
		{strings.Repeat("a", 40), strings.Repeat("a", 40)},
	} {
		if got := CleanSlug(c.in); got != c.want {
			t.Errorf("CleanSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
