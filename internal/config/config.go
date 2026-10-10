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
}

// Load reads settings through getenv and reports every problem at once.
func Load(getenv func(string) string) (*Config, error) {
	c := &Config{
		Addr:         or(getenv("FOYR_ADDR"), ":8080"),
		GitHubAPIURL: strings.TrimRight(or(getenv("FOYR_GITHUB_API_URL"), "https://api.github.com"), "/"),
	}
	var errs []error

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
