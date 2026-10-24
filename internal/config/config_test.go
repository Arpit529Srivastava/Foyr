package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testKeyPEM(t *testing.T) []byte {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
}

type env map[string]string

func (e env) get(k string) string { return e[k] }

func TestLoad(t *testing.T) {
	keyPEM := testKeyPEM(t)
	keyPath := filepath.Join(t.TempDir(), "app.pem")
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	valid := func() env {
		return env{
			"FOYR_GITHUB_APP_ID":           "123456",
			"FOYR_GITHUB_WEBHOOK_SECRET":   "a-long-random-webhook-secret",
			"FOYR_GITHUB_PRIVATE_KEY_PATH": keyPath,
			"FOYR_DATABASE_URL":            "postgres://foyr@localhost:5432/foyr_dev?sslmode=disable",
			"FOYR_TENANT_SUBNETS":          "subnet-a, subnet-b",
			"FOYR_TENANT_SECURITY_GROUP":   "sg-1",
			"FOYR_TENANT_EXEC_ROLE_ARN":    "arn:aws:iam::1:role/foyr-exec-tenant",
			"FOYR_APPS_NAMESPACE_ID":       "ns-1",
			"FOYR_INTERNAL_TOKEN":          "internal-token-internal-token-123456",
		}
	}

	tests := []struct {
		name    string
		edit    func(env)
		wantErr []string
	}{
		{name: "all set", edit: func(env) {}},
		{name: "key as text with escaped newlines", edit: func(e env) {
			delete(e, "FOYR_GITHUB_PRIVATE_KEY_PATH")
			e["FOYR_GITHUB_PRIVATE_KEY"] = strings.ReplaceAll(string(keyPEM), "\n", `\n`)
		}},
		{name: "missing app id", edit: func(e env) { delete(e, "FOYR_GITHUB_APP_ID") }, wantErr: []string{"FOYR_GITHUB_APP_ID is not set"}},
		{name: "bad app id", edit: func(e env) { e["FOYR_GITHUB_APP_ID"] = "abc" }, wantErr: []string{"positive number"}},
		{name: "zero app id", edit: func(e env) { e["FOYR_GITHUB_APP_ID"] = "0" }, wantErr: []string{"positive number"}},
		{name: "missing secret", edit: func(e env) { delete(e, "FOYR_GITHUB_WEBHOOK_SECRET") }, wantErr: []string{"FOYR_GITHUB_WEBHOOK_SECRET is not set"}},
		{name: "short secret", edit: func(e env) { e["FOYR_GITHUB_WEBHOOK_SECRET"] = "short" }, wantErr: []string{"at least 16"}},
		{name: "missing key", edit: func(e env) { delete(e, "FOYR_GITHUB_PRIVATE_KEY_PATH") }, wantErr: []string{"FOYR_GITHUB_PRIVATE_KEY_PATH"}},
		{name: "key file not found", edit: func(e env) { e["FOYR_GITHUB_PRIVATE_KEY_PATH"] = "/nope.pem" }, wantErr: []string{"no such file"}},
		{name: "broken key", edit: func(e env) {
			delete(e, "FOYR_GITHUB_PRIVATE_KEY_PATH")
			e["FOYR_GITHUB_PRIVATE_KEY"] = "not a key"
		}, wantErr: []string{"not a PEM private key"}},
		{name: "missing database url", edit: func(e env) { delete(e, "FOYR_DATABASE_URL") }, wantErr: []string{"FOYR_DATABASE_URL is not set"}},
		{name: "database url not postgres", edit: func(e env) { e["FOYR_DATABASE_URL"] = "mysql://x" }, wantErr: []string{"must start with postgres://"}},
		{name: "bad workers", edit: func(e env) { e["FOYR_WORKERS"] = "lots" }, wantErr: []string{"FOYR_WORKERS"}},
		{name: "negative workers", edit: func(e env) { e["FOYR_WORKERS"] = "-1" }, wantErr: []string{"FOYR_WORKERS"}},
		{name: "runtime settings needed with workers", edit: func(e env) { delete(e, "FOYR_TENANT_SECURITY_GROUP"); delete(e, "FOYR_APPS_NAMESPACE_ID") },
			wantErr: []string{"FOYR_TENANT_SECURITY_GROUP is not set", "FOYR_APPS_NAMESPACE_ID is not set"}},
		{name: "missing internal token", edit: func(e env) { delete(e, "FOYR_INTERNAL_TOKEN") }, wantErr: []string{"FOYR_INTERNAL_TOKEN is not set"}},
		{name: "short internal token", edit: func(e env) { e["FOYR_INTERNAL_TOKEN"] = "short" }, wantErr: []string{"at least 32"}},
		{name: "founder key and apps domain", edit: func(e env) {
			e["FOYR_FOUNDER_KEY"] = strings.Repeat("f", 32)
			e["FOYR_APPS_DOMAIN"] = "Apps.Foyr.Online"
		}},
		{name: "short founder key", edit: func(e env) { e["FOYR_FOUNDER_KEY"] = "short" }, wantErr: []string{"FOYR_FOUNDER_KEY must be at least 32"}},
		{name: "bad capacity", edit: func(e env) { e["FOYR_APPS_CAPACITY"] = "EC2" }, wantErr: []string{"FOYR_APPS_CAPACITY"}},
		{name: "everything missing is reported together", edit: func(e env) { clear(e) }, wantErr: []string{
			"FOYR_GITHUB_APP_ID is not set", "FOYR_GITHUB_WEBHOOK_SECRET is not set", "FOYR_GITHUB_PRIVATE_KEY_PATH", "FOYR_DATABASE_URL is not set",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := valid()
			tt.edit(e)
			c, err := Load(e.get)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if c.GitHubAppID != 123456 || c.GitHubPrivateKey == nil || c.Addr != ":8080" || c.GitHubAPIURL != "https://api.github.com" ||
					c.Workers != 2 || c.AWSRegion != "ap-south-1" || c.CodeBuildProject != "foyr-build" ||
					len(c.TenantSubnets) != 2 || c.TenantSubnets[1] != "subnet-b" || c.ECSCluster != "foyr" ||
					c.AppsLogGroup != "/foyr/apps" || c.AppsCapacity != "FARGATE_SPOT" {
					t.Fatalf("unexpected config: %+v", c)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

func TestErrorsNeverContainKeyMaterial(t *testing.T) {
	secret := "-----BEGIN RSA PRIVATE KEY-----\nTOPSECRETMATERIAL\n-----END RSA PRIVATE KEY-----"
	_, err := Load(env{"FOYR_GITHUB_PRIVATE_KEY": secret}.get)
	if err == nil || strings.Contains(err.Error(), "TOPSECRET") {
		t.Fatalf("error leaked key material or was nil: %v", err)
	}
}

func TestWorkersCanBeTurnedOff(t *testing.T) {
	e := env{
		"FOYR_GITHUB_APP_ID":         "1",
		"FOYR_GITHUB_WEBHOOK_SECRET": "a-long-random-webhook-secret",
		"FOYR_GITHUB_PRIVATE_KEY":    string(testKeyPEM(t)),
		"FOYR_DATABASE_URL":          "postgres://x",
		"FOYR_WORKERS":               "0",
		"FOYR_INTERNAL_TOKEN":        "internal-token-internal-token-123456",
	}
	c, err := Load(e.get)
	if err != nil || c.Workers != 0 {
		t.Fatalf("workers = %v err=%v", c, err)
	}
}

func TestFounderSettings(t *testing.T) {
	e := env{
		"FOYR_GITHUB_APP_ID":         "1",
		"FOYR_GITHUB_WEBHOOK_SECRET": "a-long-random-webhook-secret",
		"FOYR_GITHUB_PRIVATE_KEY":    string(testKeyPEM(t)),
		"FOYR_DATABASE_URL":          "postgres://x",
		"FOYR_INTERNAL_TOKEN":        "internal-token-internal-token-123456",
		"FOYR_WORKERS":               "0",
		"FOYR_FOUNDER_KEY":           strings.Repeat("f", 32),
		"FOYR_APPS_DOMAIN":           "Apps.Foyr.Online.",
	}
	c, err := Load(e.get)
	if err != nil || c.FounderKey != strings.Repeat("f", 32) || c.AppsDomain != "apps.foyr.online" {
		t.Fatalf("config = %+v err=%v", c, err)
	}
}

func loginEnv(t *testing.T) env {
	t.Helper()
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	return env{
		"FOYR_GITHUB_APP_ID":         "1",
		"FOYR_GITHUB_WEBHOOK_SECRET": "a-long-random-webhook-secret",
		"FOYR_GITHUB_PRIVATE_KEY":    string(testKeyPEM(t)),
		"FOYR_DATABASE_URL":          "postgres://x",
		"FOYR_INTERNAL_TOKEN":        "internal-token-internal-token-123456",
		"FOYR_WORKERS":               "0",
		"FOYR_APPS_DOMAIN":           "apps.foyr.online",
		"FOYR_SESSION_KEY":           base64.StdEncoding.EncodeToString(seed),
		"FOYR_AUTH_URL":              "https://auth.foyr.online/",
		"FOYR_RESEND_API_KEY":        "re_test_key",
		"FOYR_MAIL_FROM":             "Foyr <login@foyr.online>",
	}
}

func TestLoginSettings(t *testing.T) {
	c, err := Load(loginEnv(t).get)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.SessionKey) != ed25519.PrivateKeySize || c.AuthURL != "https://auth.foyr.online" ||
		c.ResendAPIKey != "re_test_key" || c.MailFrom != "Foyr <login@foyr.online>" || !c.LoginOn() {
		t.Fatalf("config = %+v", c)
	}

	// Without a session key login is off and the other settings are not checked.
	off := loginEnv(t)
	delete(off, "FOYR_SESSION_KEY")
	off["FOYR_AUTH_URL"] = "http://evil.com"
	if c, err := Load(off.get); err != nil || c.LoginOn() {
		t.Fatalf("login off: %+v, %v", c, err)
	}

	// Local development: localhost over http and no Resend key (emails go to the log).
	dev := loginEnv(t)
	dev["FOYR_AUTH_URL"] = "http://localhost:8080"
	delete(dev, "FOYR_RESEND_API_KEY")
	delete(dev, "FOYR_MAIL_FROM")
	if c, err := Load(dev.get); err != nil || !c.LoginOn() || c.ResendAPIKey != "" {
		t.Fatalf("dev: %+v, %v", c, err)
	}
}

func TestLoginSettingsRejected(t *testing.T) {
	cases := []struct {
		name    string
		edit    func(env)
		wantErr string
	}{
		{"bad key", func(e env) { e["FOYR_SESSION_KEY"] = "not-a-key" }, "FOYR_SESSION_KEY"},
		{"short key", func(e env) { e["FOYR_SESSION_KEY"] = base64.StdEncoding.EncodeToString([]byte("short")) }, "FOYR_SESSION_KEY"},
		{"no auth url", func(e env) { delete(e, "FOYR_AUTH_URL") }, "FOYR_AUTH_URL is not set"},
		{"plain http on a real host", func(e env) { e["FOYR_AUTH_URL"] = "http://evil.com" }, "FOYR_AUTH_URL must be https"},
		{"localhost lookalike", func(e env) { e["FOYR_AUTH_URL"] = "http://localhost.evil.com" }, "FOYR_AUTH_URL must be https"},
		{"auth url with a path", func(e env) { e["FOYR_AUTH_URL"] = "https://auth.foyr.online/login" }, "FOYR_AUTH_URL must be https"},
		{"no mail setup", func(e env) { delete(e, "FOYR_RESEND_API_KEY") }, "FOYR_RESEND_API_KEY"},
		{"resend without a from address", func(e env) { delete(e, "FOYR_MAIL_FROM") }, "FOYR_MAIL_FROM"},
		{"no apps domain", func(e env) { delete(e, "FOYR_APPS_DOMAIN") }, "FOYR_APPS_DOMAIN"},
	}
	for _, c := range cases {
		e := loginEnv(t)
		c.edit(e)
		_, err := Load(e.get)
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: err = %v, want it to mention %q", c.name, err, c.wantErr)
		}
		if err != nil && strings.Contains(err.Error(), e["FOYR_SESSION_KEY"]) && e["FOYR_SESSION_KEY"] != "" {
			t.Errorf("%s: error leaks the session key: %v", c.name, err)
		}
	}
}
