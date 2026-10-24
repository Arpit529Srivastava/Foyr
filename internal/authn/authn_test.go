package authn

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func testKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestSignVerify(t *testing.T) {
	pub, priv := testKeys(t)
	otherPub, otherPriv := testKeys(t)
	now := time.Unix(1_800_000_000, 0)
	c := Claims{SID: "s1", UserID: 7, Email: "ana@example.com", Slug: "crm", Exp: now.Add(time.Hour).Unix()}
	tok := Sign(priv, c)

	got, err := Verify(pub, tok, now)
	if err != nil || got != c {
		t.Fatalf("round trip = %+v, %v; want %+v", got, err, c)
	}

	parts := strings.Split(tok, ".")
	flip := func(s string) string {
		b := []byte(s)
		if b[2] == 'A' {
			b[2] = 'B'
		} else {
			b[2] = 'A'
		}
		return string(b)
	}
	cases := map[string]string{
		"payload changed":   parts[0] + "." + flip(parts[1]) + "." + parts[2],
		"signature changed": parts[0] + "." + parts[1] + "." + flip(parts[2]),
		"other key":         Sign(otherPriv, c),
		"v2 prefix":         "v2." + parts[1] + "." + parts[2],
		"empty":             "",
		"two parts":         parts[0] + "." + parts[1],
		"not base64":        "v1.!!!.???",
	}
	for name, bad := range cases {
		if _, err := Verify(pub, bad, now); !errors.Is(err, ErrBadToken) {
			t.Errorf("%s: err = %v, want ErrBadToken", name, err)
		}
	}
	if _, err := Verify(otherPub, tok, now); !errors.Is(err, ErrBadToken) {
		t.Errorf("wrong public key: err = %v, want ErrBadToken", err)
	}
	if _, err := Verify(pub, tok, now.Add(2*time.Hour)); !errors.Is(err, ErrExpired) {
		t.Errorf("past exp: err = %v, want ErrExpired", err)
	}
}

func TestVerifyKeepsClaimsJSONKeys(t *testing.T) {
	_, priv := testKeys(t)
	tok := Sign(priv, Claims{SID: "s", UserID: 1, Email: "a@b.c", Slug: "x", Exp: 5})
	raw, err := base64.RawURLEncoding.DecodeString(strings.Split(tok, ".")[1])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"sid", "uid", "email", "app", "exp"} {
		if _, ok := m[k]; !ok {
			t.Errorf("payload lacks key %q: %s", k, raw)
		}
	}
}

func TestSafeReturn(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/reports?x=1", "/reports?x=1"},
		{"/", "/"},
		{"", "/"},
		{"reports", "/"},
		{"//evil.com", "/"},
		{"/\\evil.com", "/"},
		{"https://evil.com", "/"},
		{"/%2F%2Fevil.com", "/"},
		{"/%5Cevil.com", "/"},
		{"/a\r\nSet-Cookie:x", "/"},
		{"/a%0d%0aSet-Cookie:x", "/"},
		{"/%zz", "/"},
	}
	for _, c := range cases {
		if got := SafeReturn(c.in); got != c.want {
			t.Errorf("SafeReturn(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got, ok := NormalizeEmail(" Arpit@Gmail.COM "); !ok || got != "arpit@gmail.com" {
		t.Errorf("got %q, %v", got, ok)
	}
	for _, bad := range []string{"a@b", "a b@c.d", "@c.d", "a@@c.d", "", "a@.d", strings.Repeat("a", 250) + "@b.cd"} {
		if got, ok := NormalizeEmail(bad); ok {
			t.Errorf("NormalizeEmail(%q) = %q, true; want false", bad, got)
		}
	}
}

func TestNewSecret(t *testing.T) {
	a, ha := NewSecret()
	b, _ := NewSecret()
	if a == b {
		t.Error("two secrets are equal")
	}
	if len(a) != 43 {
		t.Errorf("len(raw) = %d, want 43", len(a))
	}
	want := sha256.Sum256([]byte(a))
	if string(ha) != string(want[:]) || string(Hash(a)) != string(ha) {
		t.Error("hash is not SHA-256 of the raw secret")
	}
}

func TestParseKeys(t *testing.T) {
	seed := make([]byte, 32)
	_, _ = rand.Read(seed)
	priv, err := ParsePrivateKey(base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ParsePublicKey(base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(pub, Sign(priv, Claims{Exp: 10}), time.Unix(0, 0)); err != nil {
		t.Errorf("parsed keys do not match: %v", err)
	}
	for _, bad := range []string{"", "not base64!", base64.StdEncoding.EncodeToString(seed[:31])} {
		if _, err := ParsePrivateKey(bad); err == nil {
			t.Errorf("ParsePrivateKey(%q) accepted", bad)
		}
		if _, err := ParsePublicKey(bad); err == nil {
			t.Errorf("ParsePublicKey(%q) accepted", bad)
		}
	}
}
