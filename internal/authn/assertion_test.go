package authn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	testIss = "https://auth.example.test"
	testAud = "my-app"
	testSub = "usr_0123456789abcdef"
)

var t0 = time.Unix(1_800_000_000, 0)

func newPriv(t testing.TB) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func goodClaims() AssertionClaims {
	return NewAssertion(testIss, testAud, testSub, "ana@example.com", "member", t0)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func mustJSON(t testing.TB, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// forge signs arbitrary header and claims with key, as a hostile or buggy issuer might.
func forge(t testing.TB, key *ecdsa.PrivateKey, header map[string]any, c AssertionClaims) string {
	t.Helper()
	input := b64(mustJSON(t, header)) + "." + b64(mustJSON(t, c))
	sum := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + b64(sig)
}

func hdr(key *ecdsa.PrivateKey) map[string]any {
	return map[string]any{"alg": "ES256", "typ": "JWT", "kid": KeyID(&key.PublicKey)}
}

func TestAssertionRoundTripOddEmails(t *testing.T) {
	key := newPriv(t)
	keys := KeySet([]*ecdsa.PublicKey{&key.PublicKey})
	for _, email := range []string{`o'brien+x@example.com`, `"quoted"@example.com`, "ana@例え.jp"} {
		c := NewAssertion(testIss, testAud, testSub, email, "admin", t0)
		tok, err := SignAssertion(key, c)
		if err != nil {
			t.Fatal(err)
		}
		got, err := VerifyAssertion(keys, tok, testIss, testAud, t0)
		if err != nil || got.Email != email || got != c {
			t.Fatalf("%q: got %+v err %v", email, got, err)
		}
	}
}

func TestSignatureIsAlways64Bytes(t *testing.T) {
	key := newPriv(t)
	c := goodClaims()
	for i := 0; i < 2000; i++ {
		tok, err := SignAssertion(key, c)
		if err != nil {
			t.Fatal(err)
		}
		sig, err := base64.RawURLEncoding.DecodeString(tok[strings.LastIndex(tok, ".")+1:])
		if err != nil || len(sig) != 64 {
			t.Fatalf("signature length %d err %v", len(sig), err)
		}
	}
}

func TestSignAssertionNilKey(t *testing.T) {
	if _, err := SignAssertion(nil, goodClaims()); err == nil {
		t.Fatal("nil key accepted")
	}
}

func TestVerifyRejects(t *testing.T) {
	key := newPriv(t)
	other := newPriv(t)
	keys := KeySet([]*ecdsa.PublicKey{&key.PublicKey})
	good := goodClaims()
	goodTok, _ := SignAssertion(key, good)
	parts := strings.Split(goodTok, ".")
	sigBytes, _ := base64.RawURLEncoding.DecodeString(parts[2])

	tampered := good
	tampered.Role = "owner"
	with := func(f func(c *AssertionClaims)) AssertionClaims { c := good; f(&c); return c }

	// HS256 token keyed with the public key bytes as secret.
	hsInput := b64(mustJSON(t, map[string]any{"alg": "HS256", "typ": "JWT", "kid": KeyID(&key.PublicKey)})) + "." + b64(mustJSON(t, good))
	mac := hmac.New(sha256.New, []byte(JWKS([]*ecdsa.PublicKey{&key.PublicKey})))
	mac.Write([]byte(hsInput))
	hsTok := hsInput + "." + b64(mac.Sum(nil))
	noneTok := b64(mustJSON(t, map[string]any{"alg": "none", "typ": "JWT", "kid": KeyID(&key.PublicKey)})) + "." + b64(mustJSON(t, good)) + "." + b64(sigBytes)

	critHdr := hdr(key)
	critHdr["crit"] = []string{"exp"}
	unknownKid := hdr(key)
	unknownKid["kid"] = KeyID(&other.PublicKey)

	tests := []struct {
		name  string
		token string
		now   time.Time
		want  error
	}{
		{"tampered payload", parts[0] + "." + b64(mustJSON(t, tampered)) + "." + parts[2], t0, ErrAssertionSignature},
		{"alg none", noneTok, t0, ErrAssertionAlg},
		{"alg HS256", hsTok, t0, ErrAssertionAlg},
		{"crit present", forge(t, key, critHdr, good), t0, ErrAssertionAlg},
		{"unknown kid", forge(t, other, unknownKid, good), t0, ErrAssertionKey},
		{"two segments", parts[0] + "." + parts[1], t0, ErrAssertionMalformed},
		{"four segments", goodTok + ".x", t0, ErrAssertionMalformed},
		{"padding", parts[0] + "." + parts[1] + "." + parts[2] + "==", t0, ErrAssertionMalformed},
		{"bad base64", parts[0] + "." + parts[1] + ".!!!", t0, ErrAssertionMalformed},
		{"bad header base64", "!!!." + parts[1] + "." + parts[2], t0, ErrAssertionMalformed},
		{"signature 63 bytes", parts[0] + "." + parts[1] + "." + b64(sigBytes[:63]), t0, ErrAssertionMalformed},
		{"signature 65 bytes", parts[0] + "." + parts[1] + "." + b64(append(append([]byte{}, sigBytes...), 0)), t0, ErrAssertionMalformed},
		{"wrong iss", forge(t, key, hdr(key), with(func(c *AssertionClaims) { c.Iss = "https://evil.test" })), t0, ErrAssertionIssuer},
		{"wrong aud", forge(t, key, hdr(key), with(func(c *AssertionClaims) { c.Aud = "other-app" })), t0, ErrAssertionAudience},
		{"expired", goodTok, time.Unix(good.Exp+30, 0), ErrAssertionExpired},
		{"not yet valid", goodTok, time.Unix(good.Iat-31, 0), ErrAssertionNotYetValid},
		{"role root", forge(t, key, hdr(key), with(func(c *AssertionClaims) { c.Role = "root" })), t0, ErrAssertionClaims},
		{"lifetime 301", forge(t, key, hdr(key), with(func(c *AssertionClaims) { c.Exp = c.Iat + 301 })), t0, ErrAssertionClaims},
		{"exp equals iat", forge(t, key, hdr(key), with(func(c *AssertionClaims) { c.Exp = c.Iat })), t0, ErrAssertionClaims},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := VerifyAssertion(keys, tc.token, testIss, testAud, tc.now)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := VerifyAssertion(keys, goodTok, testIss, testAud, t0); err != nil {
		t.Fatalf("control token rejected: %v", err)
	}
}

func TestVerifyLeewayBoundaries(t *testing.T) {
	key := newPriv(t)
	keys := KeySet([]*ecdsa.PublicKey{&key.PublicKey})
	c := goodClaims()
	tok, _ := SignAssertion(key, c)
	at := func(unix int64) error {
		_, err := VerifyAssertion(keys, tok, testIss, testAud, time.Unix(unix, 0))
		return err
	}
	if err := at(c.Exp + 29); err != nil {
		t.Errorf("exp+29: %v", err)
	}
	if err := at(c.Iat - 29); err != nil {
		t.Errorf("iat-29: %v", err)
	}
	if err := at(c.Exp + 31); !errors.Is(err, ErrAssertionExpired) {
		t.Errorf("exp+31: %v", err)
	}
	if err := at(c.Iat - 31); !errors.Is(err, ErrAssertionNotYetValid) {
		t.Errorf("iat-31: %v", err)
	}
}

func TestVerifyAcrossRotation(t *testing.T) {
	a, b := newPriv(t), newPriv(t)
	tokA, _ := SignAssertion(a, goodClaims())
	tokB, _ := SignAssertion(b, goodClaims())
	both := KeySet([]*ecdsa.PublicKey{&a.PublicKey, &b.PublicKey})
	for _, tok := range []string{tokA, tokB} {
		if _, err := VerifyAssertion(both, tok, testIss, testAud, t0); err != nil {
			t.Fatalf("with both keys: %v", err)
		}
	}
	onlyB := KeySet([]*ecdsa.PublicKey{&b.PublicKey})
	if _, err := VerifyAssertion(onlyB, tokA, testIss, testAud, t0); !errors.Is(err, ErrAssertionKey) {
		t.Fatalf("old key removed: %v", err)
	}
}

func TestAssertionStaysSmall(t *testing.T) {
	key := newPriv(t)
	email := strings.Repeat("a", 242) + "@example.com"
	if len(email) != 254 {
		t.Fatalf("email length %d", len(email))
	}
	c := NewAssertion("https://auth.foyr.online", strings.Repeat("s", 63), testSub, email, "member", t0)
	tok, err := SignAssertion(key, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) >= 1024 {
		t.Fatalf("token is %d bytes", len(tok))
	}
}

func BenchmarkSignAssertion(b *testing.B) {
	key := newPriv(b)
	c := goodClaims()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := SignAssertion(key, c); err != nil {
			b.Fatal(err)
		}
	}
}
