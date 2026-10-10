package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
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
