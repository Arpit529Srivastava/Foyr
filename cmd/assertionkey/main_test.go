package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
)

func parseOutput(t *testing.T, out string) (key, pub string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "key=") || !strings.HasPrefix(lines[1], "public=") {
		t.Fatalf("want key= and public= lines, got %d lines", len(lines))
	}
	return strings.TrimPrefix(lines[0], "key="), strings.TrimPrefix(lines[1], "public=")
}

func TestOutputIsAUsablePair(t *testing.T) {
	key, pub := parseOutput(t, output())
	priv, err := authn.ParseAssertionKey(key)
	if err != nil {
		t.Fatalf("key does not parse: %v", err)
	}
	pubs, err := authn.ParseAssertionPublicKeys(pub)
	if err != nil {
		t.Fatalf("public does not parse: %v", err)
	}
	now := time.Now()
	tok, err := authn.SignAssertion(priv, authn.NewAssertion("iss", "aud", "sub", "a@b.c", "member", now))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authn.VerifyAssertion(authn.KeySet(pubs), tok, "iss", "aud", now); err != nil {
		t.Fatalf("token does not verify: %v", err)
	}
	key2, _ := parseOutput(t, output())
	if key == key2 {
		t.Fatal("two calls gave the same key")
	}
}
