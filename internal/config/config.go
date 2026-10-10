// Package config loads foyrd settings from environment variables.
package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// minSecretLen is the shortest webhook secret accepted; short secrets are guessable.
const minSecretLen = 16

// Config holds everything foyrd needs at startup.
type Config struct {
	Addr                string
	GitHubAppID         int64
	GitHubPrivateKey    *rsa.PrivateKey
	GitHubWebhookSecret []byte
	GitHubAPIURL        string
	DatabaseURL         string
	InternalAddr        string // private listener for authproxy; never behind the public load balancer
	InternalToken       string // shared with authproxy
	AWSRegion           string
	CodeBuildProject    string
	Workers             int // build workers in this process; 0 turns building off

	// Where apps run (ADR-029); required when Workers > 0. Values come from `tofu output`.
	ECSCluster          string
	TenantSubnets       []string
	TenantSecurityGroup string
	TenantExecRoleARN   string
	AppsNamespaceID     string
	AppsLogGroup        string
	AppsCapacity        string // FARGATE or FARGATE_SPOT
}

// Load reads settings through getenv and reports every problem at once.
func Load(getenv func(string) string) (*Config, error) {
	c := &Config{
		Addr:         or(getenv("FOYR_ADDR"), ":8080"),
		GitHubAPIURL: strings.TrimRight(or(getenv("FOYR_GITHUB_API_URL"), "https://api.github.com"), "/"),
	}
	c.InternalAddr = or(getenv("FOYR_INTERNAL_ADDR"), ":9090")
	c.AWSRegion = or(getenv("FOYR_AWS_REGION"), "ap-south-1")
	c.CodeBuildProject = or(getenv("FOYR_CODEBUILD_PROJECT"), "foyr-build")
	var errs []error

	c.Workers = 2
	if v := getenv("FOYR_WORKERS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 64 {
			errs = append(errs, fmt.Errorf("FOYR_WORKERS must be a number from 0 to 64, got %q", v))
		}
		c.Workers = n
	}

	switch v := getenv("FOYR_GITHUB_APP_ID"); {
	case v == "":
		errs = append(errs, missing("FOYR_GITHUB_APP_ID"))
	default:
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			errs = append(errs, fmt.Errorf("FOYR_GITHUB_APP_ID must be a positive number, got %q", v))
		}
		c.GitHubAppID = id
	}

	switch v := getenv("FOYR_GITHUB_WEBHOOK_SECRET"); {
	case v == "":
		errs = append(errs, missing("FOYR_GITHUB_WEBHOOK_SECRET"))
	case len(v) < minSecretLen:
		errs = append(errs, fmt.Errorf("FOYR_GITHUB_WEBHOOK_SECRET must be at least %d characters", minSecretLen))
	default:
		c.GitHubWebhookSecret = []byte(v)
	}

	switch v := getenv("FOYR_DATABASE_URL"); {
	case v == "":
		errs = append(errs, missing("FOYR_DATABASE_URL"))
	case !strings.HasPrefix(v, "postgres://") && !strings.HasPrefix(v, "postgresql://"):
		errs = append(errs, errors.New("FOYR_DATABASE_URL must start with postgres://"))
	default:
		c.DatabaseURL = v
	}

	c.ECSCluster = or(getenv("FOYR_ECS_CLUSTER"), "foyr")
	c.AppsLogGroup = or(getenv("FOYR_APPS_LOG_GROUP"), "/foyr/apps")
	c.AppsCapacity = or(getenv("FOYR_APPS_CAPACITY"), "FARGATE_SPOT")
	if c.AppsCapacity != "FARGATE" && c.AppsCapacity != "FARGATE_SPOT" {
		errs = append(errs, fmt.Errorf("FOYR_APPS_CAPACITY must be FARGATE or FARGATE_SPOT, got %q", c.AppsCapacity))
	}
	for _, s := range strings.Split(getenv("FOYR_TENANT_SUBNETS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.TenantSubnets = append(c.TenantSubnets, s)
		}
	}
	c.TenantSecurityGroup = getenv("FOYR_TENANT_SECURITY_GROUP")
	c.TenantExecRoleARN = getenv("FOYR_TENANT_EXEC_ROLE_ARN")
	c.AppsNamespaceID = getenv("FOYR_APPS_NAMESPACE_ID")
	if c.Workers > 0 {
		for name, v := range map[string]bool{
			"FOYR_TENANT_SUBNETS":        len(c.TenantSubnets) > 0,
			"FOYR_TENANT_SECURITY_GROUP": c.TenantSecurityGroup != "",
			"FOYR_TENANT_EXEC_ROLE_ARN":  c.TenantExecRoleARN != "",
			"FOYR_APPS_NAMESPACE_ID":     c.AppsNamespaceID != "",
		} {
			if !v {
				errs = append(errs, fmt.Errorf("%s is not set (needed to run apps; scripts/infra-env.sh writes it, or set FOYR_WORKERS=0)", name))
			}
		}
	}

	switch v := getenv("FOYR_INTERNAL_TOKEN"); {
	case v == "":
		errs = append(errs, missing("FOYR_INTERNAL_TOKEN"))
	case len(v) < 32:
		errs = append(errs, errors.New("FOYR_INTERNAL_TOKEN must be at least 32 characters"))
	default:
		c.InternalToken = v
	}

	key, err := loadKey(getenv)
	if err != nil {
		errs = append(errs, err)
	}
	c.GitHubPrivateKey = key

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return c, nil
}

// loadKey reads the App key from FOYR_GITHUB_PRIVATE_KEY (PEM text) or FOYR_GITHUB_PRIVATE_KEY_PATH (file).
func loadKey(getenv func(string) string) (*rsa.PrivateKey, error) {
	if v := getenv("FOYR_GITHUB_PRIVATE_KEY"); v != "" {
		key, err := ParsePrivateKey([]byte(strings.ReplaceAll(v, `\n`, "\n")))
		if err != nil {
			return nil, fmt.Errorf("FOYR_GITHUB_PRIVATE_KEY: %w", err)
		}
		return key, nil
	}
	path := getenv("FOYR_GITHUB_PRIVATE_KEY_PATH")
	if path == "" {
		return nil, missing("FOYR_GITHUB_PRIVATE_KEY_PATH (or FOYR_GITHUB_PRIVATE_KEY)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("FOYR_GITHUB_PRIVATE_KEY_PATH: %w", err)
	}
	key, err := ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("FOYR_GITHUB_PRIVATE_KEY_PATH %s: %w", path, err)
	}
	return key, nil
}

// ParsePrivateKey parses a PEM RSA key in PKCS#1 (GitHub's format) or PKCS#8.
func ParsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("not a PEM private key")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not RSA")
		}
		return rk, nil
	default:
		return nil, fmt.Errorf("unexpected PEM block %q", block.Type)
	}
}

func missing(name string) error { return fmt.Errorf("%s is not set", name) }

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
