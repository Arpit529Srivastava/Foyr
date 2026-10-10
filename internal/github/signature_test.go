package github

import "testing"

func TestVerifySignature(t *testing.T) {
	secret := []byte("a-long-random-webhook-secret")
	body := []byte(`{"zen":"hello"}`)
	good := Sign(secret, body)

	tests := []struct {
		name   string
		secret []byte
		body   []byte
		header string
		want   bool
	}{
		{"valid", secret, body, good, true},
		{"wrong secret", []byte("another-long-random-secret"), body, good, false},
		{"missing header", secret, body, "", false},
		{"no sha256 prefix", secret, body, good[len("sha256="):], false},
		{"sha1 style header", secret, body, "sha1=" + good[len("sha256="):], false},
		{"bad hex", secret, body, "sha256=zzzz", false},
		{"body changed by one byte", secret, []byte(`{"zen":"hellO"}`), good, false},
		{"empty secret never verifies", nil, body, Sign(nil, body), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifySignature(tt.secret, tt.body, tt.header); got != tt.want {
				t.Fatalf("VerifySignature = %v, want %v", got, tt.want)
			}
		})
	}
}
