package authn

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"
)

var (
	ErrAssertionMalformed   = errors.New("authn: assertion malformed")
	ErrAssertionAlg         = errors.New("authn: assertion algorithm not accepted")
	ErrAssertionKey         = errors.New("authn: assertion key unknown")
	ErrAssertionSignature   = errors.New("authn: assertion signature invalid")
	ErrAssertionIssuer      = errors.New("authn: assertion issuer mismatch")
	ErrAssertionAudience    = errors.New("authn: assertion audience mismatch")
	ErrAssertionExpired     = errors.New("authn: assertion expired")
	ErrAssertionNotYetValid = errors.New("authn: assertion not yet valid")
	ErrAssertionClaims      = errors.New("authn: assertion claims invalid")
)

const (
	assertionLifetime = 60
	assertionMaxLife  = 300
	assertionLeeway   = 30
)

// AssertionClaims is what an identity assertion says. Iat and Exp are Unix seconds.
type AssertionClaims struct {
	Iss   string `json:"iss"`
	Aud   string `json:"aud"`
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Role  string `json:"role"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
}

type assertionHeader struct {
	Alg  string          `json:"alg"`
	Typ  string          `json:"typ"`
	Kid  string          `json:"kid"`
	Crit json.RawMessage `json:"crit"`
}

// NewAssertion builds claims valid for 60 seconds from now.
func NewAssertion(iss, aud, sub, email, role string, now time.Time) AssertionClaims {
	iat := now.Unix()
	return AssertionClaims{Iss: iss, Aud: aud, Sub: sub, Email: email, Role: role, Iat: iat, Exp: iat + assertionLifetime}
}

// SignAssertion returns the ES256 JWS compact token for the claims.
func SignAssertion(key *ecdsa.PrivateKey, c AssertionClaims) (string, error) {
	if key == nil {
		return "", errors.New("authn: assertion key is nil")
	}
	hdr, err := json.Marshal(struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
		Kid string `json:"kid"`
	}{"ES256", "JWT", KeyID(&key.PublicKey)})
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	input := enc.EncodeToString(hdr) + "." + enc.EncodeToString(body)
	sum := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + enc.EncodeToString(sig), nil
}

// AssertionKeys maps a kid to the public key that may have signed.
type AssertionKeys map[string]*ecdsa.PublicKey

// KeySet indexes keys by their KeyID.
func KeySet(keys []*ecdsa.PublicKey) AssertionKeys {
	set := make(AssertionKeys, len(keys))
	for _, k := range keys {
		set[KeyID(k)] = k
	}
	return set
}

func verifyES256(pub *ecdsa.PublicKey, signingInput string, sig []byte) bool {
	if pub == nil || len(sig) != 64 {
		return false
	}
	sum := sha256.Sum256([]byte(signingInput))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	return ecdsa.Verify(pub, sum[:], r, s)
}

func decodeStrict(seg string, v any) error {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(seg)
	if err != nil {
		return ErrAssertionMalformed
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, isHeader := v.(*assertionHeader); isHeader {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(v); err != nil || dec.More() {
		return ErrAssertionMalformed
	}
	return nil
}

func validRole(r string) bool {
	return r == "owner" || r == "admin" || r == "member" || r == "viewer"
}

// VerifyAssertion checks a token against the key set, issuer, audience and clock.
func VerifyAssertion(keys AssertionKeys, token, issuer, audience string, now time.Time) (AssertionClaims, error) {
	var zero AssertionClaims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return zero, ErrAssertionMalformed
	}
	var h assertionHeader
	if err := decodeStrict(parts[0], &h); err != nil {
		return zero, err
	}
	if h.Alg != "ES256" || h.Crit != nil {
		return zero, ErrAssertionAlg
	}
	sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return zero, ErrAssertionMalformed
	}
	pub, ok := keys[h.Kid]
	if !ok || pub == nil {
		return zero, ErrAssertionKey
	}
	if !verifyES256(pub, parts[0]+"."+parts[1], sig) {
		return zero, ErrAssertionSignature
	}
	var c AssertionClaims
	if err := decodeStrict(parts[1], &c); err != nil {
		return zero, err
	}
	if c.Iss != issuer {
		return zero, ErrAssertionIssuer
	}
	if c.Aud != audience {
		return zero, ErrAssertionAudience
	}
	if !validRole(c.Role) || c.Exp <= c.Iat || c.Exp-c.Iat > assertionMaxLife {
		return zero, ErrAssertionClaims
	}
	n := now.Unix()
	if n >= c.Exp+assertionLeeway {
		return zero, ErrAssertionExpired
	}
	if c.Iat > n+assertionLeeway {
		return zero, ErrAssertionNotYetValid
	}
	return c, nil
}
