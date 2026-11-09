package authn

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

func TestKeyPairRoundTrip(t *testing.T) {
	key, pub := NewAssertionKeyPair()
	priv, err := ParseAssertionKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pubs, err := ParseAssertionPublicKeys(pub)
	if err != nil || len(pubs) != 1 {
		t.Fatalf("pubs=%d err=%v", len(pubs), err)
	}
	if !priv.PublicKey.Equal(pubs[0]) {
		t.Fatal("public half does not match private key")
	}
}

func TestParseRejects(t *testing.T) {
	ed, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	edPub, _ := x509.MarshalPKIXPublicKey(ed)
	edKey, _ := x509.MarshalPKCS8PrivateKey(edPriv)
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p384Pub, _ := x509.MarshalPKIXPublicKey(&p384.PublicKey)
	p384Key, _ := x509.MarshalPKCS8PrivateKey(p384)
	goodKey, goodPub := NewAssertionKeyPair()
	b := base64.StdEncoding.EncodeToString

	privBad := []string{"", "!!!not base64!!!", b(edKey), b(p384Key), goodPub}
	for _, in := range privBad {
		_, err := ParseAssertionKey(in)
		if err == nil {
			t.Errorf("ParseAssertionKey(%q) accepted", in)
		} else if in != "" && strings.Contains(err.Error(), in) {
			t.Errorf("error contains input %q", in)
		}
	}
	pubBad := []string{"", "!!!not base64!!!", b(edPub), b(p384Pub), goodKey, goodPub + ","}
	for _, in := range pubBad {
		_, err := ParseAssertionPublicKeys(in)
		if err == nil {
			t.Errorf("ParseAssertionPublicKeys(%q) accepted", in)
		} else if in != "" && strings.Contains(err.Error(), in) {
			t.Errorf("error contains input %q", in)
		}
	}
	pubs, err := ParseAssertionPublicKeys(goodPub + "," + goodPub)
	if err != nil || len(pubs) != 1 {
		t.Fatalf("duplicates: len=%d err=%v", len(pubs), err)
	}
}

func newPub(t testing.TB) *ecdsa.PublicKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &k.PublicKey
}

func TestKeyIDIsAThumbprint(t *testing.T) {
	a, b := newPub(t), newPub(t)
	x := base64.RawURLEncoding.EncodeToString(a.X.FillBytes(make([]byte, 32)))
	y := base64.RawURLEncoding.EncodeToString(a.Y.FillBytes(make([]byte, 32)))
	sum := sha256.Sum256([]byte(`{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if got := KeyID(a); got != want {
		t.Fatalf("KeyID = %s, want %s", got, want)
	}
	if KeyID(a) != KeyID(a) {
		t.Fatal("unstable")
	}
	if KeyID(a) == KeyID(b) {
		t.Fatal("different keys share an id")
	}
}

func TestJWKSShapeAndOrder(t *testing.T) {
	keys := []*ecdsa.PublicKey{newPub(t), newPub(t), newPub(t)}
	var doc struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.Unmarshal(JWKS(keys), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Keys) != 3 {
		t.Fatalf("len = %d", len(doc.Keys))
	}
	for i, k := range doc.Keys {
		if len(k) != 7 || k["kty"] != "EC" || k["crv"] != "P-256" || k["alg"] != "ES256" || k["use"] != "sig" {
			t.Fatalf("bad member %v", k)
		}
		if i > 0 && doc.Keys[i-1]["kid"] >= k["kid"] {
			t.Fatal("not sorted by kid")
		}
		for _, f := range []string{"x", "y"} {
			raw, err := base64.RawURLEncoding.DecodeString(k[f])
			if err != nil || len(raw) != 32 {
				t.Fatalf("%s is not 32 bytes", f)
			}
		}
	}
}

func TestVerifyES256AgainstRFC7515(t *testing.T) {
	dec := func(s string) []byte {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	pub := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(dec("f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU")),
		Y:     new(big.Int).SetBytes(dec("x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0")),
	}
	input := "eyJhbGciOiJFUzI1NiJ9" + "." +
		"eyJpc3MiOiJqb2UiLA0KICJleHAiOjEzMDA4MTkzODAsDQogImh0dHA6Ly9leGFt" +
		"cGxlLmNvbS9pc19yb290Ijp0cnVlfQ"
	sig := dec("DtEhU3ljbEg8L38VWAfUAqOyKAM6-Xx-F4GawxaepmXFCgfTjDxw5djxLa8ISlSA" +
		"pmWQxfKTUJqPP3-Kg6NU1Q")
	if !verifyES256(pub, input, sig) {
		t.Fatal("RFC 7515 A.3 signature rejected")
	}
	sig[10] ^= 1
	if verifyES256(pub, input, sig) {
		t.Fatal("flipped bit accepted")
	}
}
