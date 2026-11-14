package authn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

var (
	errAssertionKeyFormat  = errors.New("authn: assertion key is not a base64 PKCS#8 P-256 key")
	errAssertionPublicKeys = errors.New("authn: assertion public keys are not a comma-separated list of base64 SPKI P-256 keys")
)

// NewAssertionKeyPair returns a new P-256 pair as base64 PKCS#8 DER and base64 SPKI DER.
func NewAssertionKeyPair() (key, public string) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic("authn: key generation failed: " + err.Error())
	}
	k, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		panic("authn: key encoding failed: " + err.Error())
	}
	p, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		panic("authn: key encoding failed: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(k), base64.StdEncoding.EncodeToString(p)
}

// ParseAssertionKey parses a base64 PKCS#8 P-256 private key.
func ParseAssertionKey(b64 string) (*ecdsa.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, errAssertionKeyFormat
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, errAssertionKeyFormat
	}
	priv, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || priv.Curve != elliptic.P256() {
		return nil, errAssertionKeyFormat
	}
	return priv, nil
}

// ParseAssertionPublicKeys parses a comma-separated list of base64 SPKI P-256 keys, dropping duplicates.
func ParseAssertionPublicKeys(csv string) ([]*ecdsa.PublicKey, error) {
	var out []*ecdsa.PublicKey
	seen := map[string]bool{}
	for _, part := range strings.Split(csv, ",") {
		der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(part))
		if err != nil || len(der) == 0 {
			return nil, errAssertionPublicKeys
		}
		parsed, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			return nil, errAssertionPublicKeys
		}
		pub, ok := parsed.(*ecdsa.PublicKey)
		if !ok || pub.Curve != elliptic.P256() {
			return nil, errAssertionPublicKeys
		}
		id := KeyID(pub)
		if !seen[id] {
			seen[id] = true
			out = append(out, pub)
		}
	}
	return out, nil
}

func coord(n interface{ FillBytes([]byte) []byte }) string {
	return base64.RawURLEncoding.EncodeToString(n.FillBytes(make([]byte, 32)))
}

// KeyID is the RFC 7638 thumbprint of the key, base64url without padding.
func KeyID(pub *ecdsa.PublicKey) string {
	s := `{"crv":"P-256","kty":"EC","x":"` + coord(pub.X) + `","y":"` + coord(pub.Y) + `"}`
	sum := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// JWKS renders the keys as a JWK Set sorted by kid.
func JWKS(keys []*ecdsa.PublicKey) []byte {
	set := make([]jwk, 0, len(keys))
	for _, k := range keys {
		set = append(set, jwk{"EC", "P-256", "ES256", "sig", KeyID(k), coord(k.X), coord(k.Y)})
	}
	sort.Slice(set, func(i, j int) bool { return set[i].Kid < set[j].Kid })
	b, err := json.Marshal(struct {
		Keys []jwk `json:"keys"`
	}{set})
	if err != nil {
		panic("authn: jwks encoding failed: " + err.Error())
	}
	return b
}
