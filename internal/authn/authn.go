// Package authn holds the pieces of magic-link login that foyrd and authproxy share:
// signed session tokens, one-time secrets, safe return paths and the access snapshot.
package authn

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrBadToken means the token is malformed or its signature does not match.
	ErrBadToken = errors.New("authn: bad token")
	// ErrExpired means the token is genuine but past its expiry.
	ErrExpired = errors.New("authn: token expired")
)

const (
	tokenPrefix = "v1."
	signContext = "foyr session|"
)

// Claims is what a session token says. Exp is Unix seconds.
type Claims struct {
	SID    string `json:"sid"`
	UserID int64  `json:"uid"`
	Email  string `json:"email"`
	Slug   string `json:"app"`
	Exp    int64  `json:"exp"`
}

// Sign returns "v1.<payload>.<signature>" with the signature over a fixed context plus the payload.
func Sign(key ed25519.PrivateKey, c Claims) string {
	body, _ := json.Marshal(c)
	payload := base64.RawURLEncoding.EncodeToString(body)
	sig := ed25519.Sign(key, []byte(signContext+payload))
	return tokenPrefix + payload + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// Verify checks the signature, then the expiry against now.
func Verify(pub ed25519.PublicKey, token string, now time.Time) (Claims, error) {
	var c Claims
	rest, ok := strings.CutPrefix(token, tokenPrefix)
	if !ok {
		return c, ErrBadToken
	}
	payload, sigPart, ok := strings.Cut(rest, ".")
	if !ok || len(pub) != ed25519.PublicKeySize {
		return c, ErrBadToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigPart)
	if err != nil || !ed25519.Verify(pub, []byte(signContext+payload), sig) {
		return c, ErrBadToken
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || json.Unmarshal(body, &c) != nil {
		return Claims{}, ErrBadToken
	}
	if now.Unix() >= c.Exp {
		return Claims{}, ErrExpired
	}
	return c, nil
}

// NewSecret returns 32 random bytes as base64url, and the SHA-256 that gets stored instead.
func NewSecret() (raw string, hash []byte) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("authn: no randomness: %v", err))
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, Hash(raw)
}

// Hash is the SHA-256 of a raw secret.
func Hash(raw string) []byte {
	h := sha256.Sum256([]byte(raw))
	return h[:]
}

// SafeReturn keeps p only if it is a path on this host; anything that could leave it becomes "/".
func SafeReturn(p string) string {
	decoded, err := url.PathUnescape(p)
	if err != nil || !strings.HasPrefix(p, "/") || !strings.HasPrefix(decoded, "/") {
		return "/"
	}
	for _, s := range []string{p, decoded} {
		if strings.HasPrefix(s, "//") || strings.ContainsAny(s, "\\") || strings.IndexFunc(s, isControl) >= 0 {
			return "/"
		}
	}
	return p
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

// ParsePrivateKey reads a 32-byte Ed25519 seed in standard base64.
func ParsePrivateKey(b64seed string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64seed))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("authn: session key must be a 32-byte seed in base64")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// ParsePublicKey reads a 32-byte Ed25519 public key in standard base64.
func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("authn: session public key must be 32 bytes in base64")
	}
	return ed25519.PublicKey(b), nil
}

// AppAccess is who may open one app. Members maps user id to role.
type AppAccess struct {
	Slug    string           `json:"slug"`
	Mode    string           `json:"mode"`
	Members map[int64]string `json:"members"`
}

// AccessSnapshot is what authproxy polls: every live app's access, plus unexpired revoked session ids.
type AccessSnapshot struct {
	Apps    []AppAccess `json:"apps"`
	Revoked []string    `json:"revoked"`
}

// NormalizeEmail trims and lower-cases s; false unless it looks like one address.
func NormalizeEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) > 254 || strings.ContainsAny(s, " \t\r\n") || strings.Count(s, "@") != 1 {
		return "", false
	}
	local, domain, _ := strings.Cut(s, "@")
	dot := strings.LastIndex(domain, ".")
	if local == "" || dot < 1 || dot == len(domain)-1 {
		return "", false
	}
	return s, true
}
