// Package slug turns repository names into app hostnames: <slug>.<APPS_DOMAIN> (system design §7).
package slug

import (
	"regexp"
	"strings"
)

// valid is the shape of every slug: 3 to 40 of a-z, 0-9 and dashes, no dash at either end.
var valid = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,38})[a-z0-9]$`)

// reserved names are never given to apps, so they can't impersonate Foyr's own hosts.
var reserved = map[string]bool{
	"app": true, "api": true, "auth": true, "www": true, "docs": true, "admin": true,
	"status": true, "mail": true, "static": true, "cdn": true, "foyr": true,
}

// Valid reports whether s can be an app slug.
func Valid(s string) bool { return valid.MatchString(s) && !reserved[s] }

// From returns the preferred slug for a repository name (the part after "owner/").
func From(repo string) string {
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		repo = repo[i+1:]
	}
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(repo) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if len(s) < 3 || reserved[s] {
		s = strings.Trim(s+"-app", "-")
	}
	if !Valid(s) {
		s = "my-app"
	}
	return s
}

// Candidates lists slugs to try in order when the preferred one is taken.
func Candidates(repo string) []string {
	base := From(repo)
	out := []string{base}
	for i := 2; i <= 9; i++ {
		suffix := "-" + string(rune('0'+i))
		b := base
		if len(b)+len(suffix) > 40 {
			b = strings.Trim(b[:40-len(suffix)], "-")
		}
		out = append(out, b+suffix)
	}
	return out
}
