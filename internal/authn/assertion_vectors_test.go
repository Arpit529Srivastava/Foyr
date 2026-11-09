package authn

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// vectorsTestKey is a test-only P-256 key (base64 PKCS#8). It is never used outside tests.
const vectorsTestKey = "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgXB/krLJvQkUlhbFtISKljZ3OUuCnd2tKaTQPhGfM2yahRANCAAQUCfCIPPNGoXRCzZf5w6L5GHfQSaGw4U/lTeMBsdM+LRsqGrIAlwPP0vTP80eV1Dzj0zbdHlHziUon1c59PKmn"

const (
	vectorsPath = "../../sdk/vectors/assertion.json"
	vecIssuer   = "https://auth.foyr.online"
	vecAudience = "crm"
	vecNow      = 1700000030
	vecIssued   = 1700000000
)

type vectorValid struct {
	Name   string          `json:"name"`
	Token  string          `json:"token"`
	Claims AssertionClaims `json:"claims"`
}

type vectorInvalid struct {
	Name   string `json:"name"`
	Token  string `json:"token"`
	Reason string `json:"reason"`
}

type vectorsFile struct {
	Issuer   string          `json:"issuer"`
	Audience string          `json:"audience"`
	Now      int64           `json:"now"`
	JWKS     json.RawMessage `json:"jwks"`
	Valid    []vectorValid   `json:"valid"`
	Invalid  []vectorInvalid `json:"invalid"`
}

var assertionErrors = map[string]error{
	"ErrAssertionMalformed":   ErrAssertionMalformed,
	"ErrAssertionAlg":         ErrAssertionAlg,
	"ErrAssertionKey":         ErrAssertionKey,
	"ErrAssertionSignature":   ErrAssertionSignature,
	"ErrAssertionIssuer":      ErrAssertionIssuer,
	"ErrAssertionAudience":    ErrAssertionAudience,
	"ErrAssertionExpired":     ErrAssertionExpired,
	"ErrAssertionNotYetValid": ErrAssertionNotYetValid,
	"ErrAssertionClaims":      ErrAssertionClaims,
}

func vectorsKey(t testing.TB) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ParseAssertionKey(vectorsTestKey)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func buildVectors(t testing.TB) vectorsFile {
	t.Helper()
	key := vectorsKey(t)
	pub := &key.PublicKey
	kid := KeyID(pub)
	other := newPriv(t)
	base := NewAssertion(vecIssuer, vecAudience, testSub, "ana@example.com", "member", time.Unix(vecIssued, 0))
	with := func(f func(c *AssertionClaims)) AssertionClaims { c := base; f(&c); return c }
	sign := func(c AssertionClaims) string {
		tok, err := SignAssertion(key, c)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	goodTok := sign(base)
	parts := strings.Split(goodTok, ".")
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	resig := func(sig []byte) string { return parts[0] + "." + parts[1] + "." + b64(sig) }
	hj := `{"alg":"ES256","typ":"JWT","kid":"` + kid + `"}`
	pj := string(mustJSON(t, base))
	pjWith := func(old, repl string) string { return strings.Replace(pj, old, repl, 1) }

	// High-S twin of the good signature: (r, N-s) is still a valid ECDSA signature.
	n := elliptic.P256().Params().N
	hs := new(big.Int).Sub(n, new(big.Int).SetBytes(sigBytes[32:]))
	if hs.Cmp(new(big.Int).Rsh(n, 1)) <= 0 {
		hs.Sub(n, hs) // the original was already high-S
	}
	highS := make([]byte, 64)
	copy(highS[:32], sigBytes[:32])
	hs.FillBytes(highS[32:])

	hsInput := b64(mustJSON(t, map[string]any{"alg": "HS256", "typ": "JWT", "kid": kid})) + "." + b64(mustJSON(t, base))
	mac := hmac.New(sha256.New, JWKS([]*ecdsa.PublicKey{pub}))
	mac.Write([]byte(hsInput))
	noneTok := b64(mustJSON(t, map[string]any{"alg": "none", "typ": "JWT", "kid": kid})) + "." + b64(mustJSON(t, base)) + "." + b64(sigBytes)
	critHdr := hdr(key)
	critHdr["crit"] = []string{"exp"}
	otherHdr := hdr(key)
	otherHdr["kid"] = KeyID(&other.PublicKey)
	tampered := base
	tampered.Role = "owner"
	rs := func(r, s *big.Int) string {
		sig := make([]byte, 64)
		r.FillBytes(sig[:32])
		s.FillBytes(sig[32:])
		return resig(sig)
	}
	one := big.NewInt(1)
	odd := with(func(c *AssertionClaims) { c.Email = `o'brien+tag@exämple.com"x` })

	v := vectorsFile{Issuer: vecIssuer, Audience: vecAudience, Now: vecNow, JWKS: JWKS([]*ecdsa.PublicKey{pub})}
	addV := func(name, tok string, c AssertionClaims) { v.Valid = append(v.Valid, vectorValid{name, tok, c}) }
	addI := func(name, tok string, reason error) {
		for k, e := range assertionErrors {
			if e == reason {
				v.Invalid = append(v.Invalid, vectorInvalid{name, tok, k})
				return
			}
		}
		t.Fatalf("unnamed reason %v", reason)
	}

	addV("basic-member", goodTok, base)
	for _, role := range []string{"owner", "admin", "viewer"} {
		c := with(func(c *AssertionClaims) { c.Role = role })
		addV("role-"+role, sign(c), c)
	}
	addV("email-quotes-plus-non-ascii", sign(odd), odd)
	addV("extra-payload-claim-ignored", forgeRaw(t, key, hj, pjWith(`"role"`, `"extra":{"a":[1,{"b":null}]},"role"`)), base)
	addV("high-s-signature-accepted", resig(highS), base)
	l300 := with(func(c *AssertionClaims) { c.Exp = c.Iat + 300 })
	addV("lifetime-300s", sign(l300), l300)
	early := with(func(c *AssertionClaims) { c.Iat = vecNow + 30; c.Exp = c.Iat + 60 })
	addV("issued-30s-ahead-within-leeway", sign(early), early)
	late := with(func(c *AssertionClaims) { c.Iat = vecNow - 61; c.Exp = vecNow - 30 + 1 })
	addV("expired-29s-within-leeway", sign(late), late)

	addI("alg-none", noneTok, ErrAssertionAlg)
	addI("alg-hs256-public-key-as-secret", hsInput+"."+b64(mac.Sum(nil)), ErrAssertionAlg)
	addI("crit-header-present", forge(t, key, critHdr, base), ErrAssertionAlg)
	addI("unknown-kid", forge(t, other, otherHdr, base), ErrAssertionKey)
	addI("truncated-signature", resig(sigBytes[:63]), ErrAssertionMalformed)
	addI("signature-65-bytes", resig(append(append([]byte{}, sigBytes...), 0)), ErrAssertionMalformed)
	addI("two-segments", parts[0]+"."+parts[1], ErrAssertionMalformed)
	addI("four-segments", goodTok+".x", ErrAssertionMalformed)
	addI("padding-in-signature", goodTok+"==", ErrAssertionMalformed)
	addI("newline-in-signature", parts[0]+"."+parts[1]+"."+parts[2][:10]+"\n"+parts[2][10:], ErrAssertionMalformed)
	addI("carriage-return-in-payload", parts[0]+"."+parts[1][:10]+"\r"+parts[1][10:]+"."+parts[2], ErrAssertionMalformed)
	addI("header-member-wrong-case", forgeRaw(t, key, `{"ALG":"ES256","typ":"JWT","kid":"`+kid+`"}`, pj), ErrAssertionMalformed)
	addI("header-unknown-member", forgeRaw(t, key, `{"alg":"ES256","typ":"JWT","kid":"`+kid+`","jku":"x"}`, pj), ErrAssertionMalformed)
	addI("header-duplicate-member", forgeRaw(t, key, `{"alg":"ES256","alg":"ES256","typ":"JWT","kid":"`+kid+`"}`, pj), ErrAssertionMalformed)
	addI("header-trailing-data", forgeRaw(t, key, hj+hj, pj), ErrAssertionMalformed)
	addI("payload-duplicate-member", forgeRaw(t, key, hj, pjWith(`"role":"member"`, `"role":"viewer","role":"owner"`)), ErrAssertionMalformed)
	addI("payload-trailing-data", forgeRaw(t, key, hj, pj+"}"), ErrAssertionMalformed)
	addI("payload-not-an-object", forgeRaw(t, key, hj, `[]`), ErrAssertionMalformed)
	addI("signature-r-zero", rs(big.NewInt(0), one), ErrAssertionSignature)
	addI("signature-s-equals-n", rs(one, n), ErrAssertionSignature)
	addI("tampered-payload", parts[0]+"."+b64(mustJSON(t, tampered))+"."+parts[2], ErrAssertionSignature)
	addI("wrong-issuer", sign(with(func(c *AssertionClaims) { c.Iss = "https://evil.test" })), ErrAssertionIssuer)
	addI("wrong-audience", sign(with(func(c *AssertionClaims) { c.Aud = "other-app" })), ErrAssertionAudience)
	exp := with(func(c *AssertionClaims) { c.Iat = vecNow - 90; c.Exp = vecNow - 30 })
	addI("expired", sign(exp), ErrAssertionExpired)
	nyv := with(func(c *AssertionClaims) { c.Iat = vecNow + 31; c.Exp = c.Iat + 60 })
	addI("not-yet-valid", sign(nyv), ErrAssertionNotYetValid)
	addI("unknown-role", sign(with(func(c *AssertionClaims) { c.Role = "root" })), ErrAssertionClaims)
	addI("payload-role-upper-case", forgeRaw(t, key, hj, pjWith(`"role"`, `"ROLE"`)), ErrAssertionClaims)
	addI("lifetime-301s", sign(with(func(c *AssertionClaims) { c.Exp = c.Iat + 301 })), ErrAssertionClaims)
	addI("exp-equals-iat", sign(with(func(c *AssertionClaims) { c.Exp = c.Iat })), ErrAssertionClaims)
	addI("iat-negative", sign(with(func(c *AssertionClaims) { c.Iat = -1; c.Exp = 59 })), ErrAssertionClaims)
	addI("exp-2-pow-53", sign(with(func(c *AssertionClaims) { c.Exp = 1 << 53 })), ErrAssertionClaims)
	return v
}

func TestGenerateVectors(t *testing.T) {
	if os.Getenv("FOYR_UPDATE_VECTORS") != "1" {
		t.Skip("set FOYR_UPDATE_VECTORS=1 to rewrite " + vectorsPath)
	}
	b, err := json.MarshalIndent(buildVectors(t), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("../../sdk/vectors", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vectorsPath, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVectorsFile(t *testing.T) {
	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("vectors file missing (run FOYR_UPDATE_VECTORS=1 go test ./internal/authn -run TestGenerateVectors): %v", err)
	}
	var v vectorsFile
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if v.Issuer != vecIssuer || v.Audience != vecAudience || v.Now != vecNow {
		t.Fatalf("header fields: %q %q %d", v.Issuer, v.Audience, v.Now)
	}
	pub := &vectorsKey(t).PublicKey
	var compact bytes.Buffer
	if err := json.Compact(&compact, v.JWKS); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(compact.Bytes(), JWKS([]*ecdsa.PublicKey{pub})) {
		t.Fatalf("jwks differs from JWKS of the test key:\n%s", compact.String())
	}
	keys := KeySet([]*ecdsa.PublicKey{pub})
	now := time.Unix(v.Now, 0)
	names := map[string]bool{}
	for _, tc := range v.Valid {
		names[tc.Name] = true
		if tc.Name == "high-s-signature-accepted" {
			sig, err := base64.RawURLEncoding.DecodeString(strings.Split(tc.Token, ".")[2])
			if err != nil || len(sig) != 64 {
				t.Fatalf("high-s vector signature: %d bytes, %v", len(sig), err)
			}
			if new(big.Int).SetBytes(sig[32:]).Cmp(new(big.Int).Rsh(elliptic.P256().Params().N, 1)) <= 0 {
				t.Errorf("high-s-signature-accepted vector is not high-S")
			}
		}
		got, err := VerifyAssertion(keys, tc.Token, v.Issuer, v.Audience, now)
		if err != nil || got != tc.Claims {
			t.Errorf("valid %s: %+v %v", tc.Name, got, err)
		}
	}
	reasons := map[string]bool{}
	for _, tc := range v.Invalid {
		names[tc.Name] = true
		reasons[tc.Reason] = true
		want, ok := assertionErrors[tc.Reason]
		if !ok {
			t.Errorf("invalid %s: unknown reason %q", tc.Name, tc.Reason)
			continue
		}
		if _, err := VerifyAssertion(keys, tc.Token, v.Issuer, v.Audience, now); !errors.Is(err, want) {
			t.Errorf("invalid %s: got %v, want %s", tc.Name, err, tc.Reason)
		}
	}
	var missing []string
	for name := range assertionErrors {
		if !reasons[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("no vector for reasons: %v", missing)
	}
	for _, name := range []string{"alg-none", "alg-hs256-public-key-as-secret", "unknown-kid", "truncated-signature", "signature-65-bytes", "lifetime-301s", "unknown-role", "tampered-payload", "wrong-audience", "wrong-issuer", "expired", "not-yet-valid", "high-s-signature-accepted", "extra-payload-claim-ignored", "header-unknown-member", "header-duplicate-member", "payload-duplicate-member", "header-member-wrong-case", "payload-role-upper-case", "iat-negative", "exp-2-pow-53"} {
		if !names[name] {
			t.Errorf("missing named vector %s", name)
		}
	}
}
