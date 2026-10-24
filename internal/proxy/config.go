package proxy

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Config is authproxy's settings, from environment variables.
type Config struct {
	Addr         string
	AppsDomain   string            // FOYR_APPS_DOMAIN, e.g. apps.foyr.online (ADR-028)
	RoutesURL    string            // foyrd's /internal/routes
	Token        string            // FOYR_INTERNAL_TOKEN, shared with foyrd
	Namespace    string            // Cloud Map domain of app tasks
	AppPort      int               // port apps listen on
	StaticRoutes map[string]string // local development instead of RoutesURL

	// Login (ADR-035): on when SessionPublicKey is set.
	FoyrdURL         string            // FOYR_FOYRD_URL: base of foyrd's internal listener; RoutesURL defaults to it plus /internal/routes
	AuthURL          string            // FOYR_AUTH_URL: where visitors sign in
	SessionPublicKey ed25519.PublicKey // FOYR_SESSION_PUBLIC_KEY: 32 bytes, base64
}

// AuthOn reports whether authproxy checks logins.
func (c *Config) AuthOn() bool { return len(c.SessionPublicKey) > 0 }

// Load reads the settings through getenv and reports every problem at once.
func Load(getenv func(string) string) (*Config, error) {
	c := &Config{
		Addr:       orDefault(getenv("FOYR_ADDR"), ":8081"),
		AppsDomain: strings.ToLower(strings.TrimSpace(getenv("FOYR_APPS_DOMAIN"))),
		RoutesURL:  getenv("FOYR_ROUTES_URL"),
		Token:      getenv("FOYR_INTERNAL_TOKEN"),
		Namespace:  orDefault(getenv("FOYR_APPS_NAMESPACE"), "apps.foyr.internal"),
		AppPort:    3000,
		FoyrdURL:   strings.TrimRight(strings.TrimSpace(getenv("FOYR_FOYRD_URL")), "/"),
	}
	if c.RoutesURL == "" && c.FoyrdURL != "" {
		c.RoutesURL = c.FoyrdURL + "/internal/routes"
	}
	var errs []error
	if c.AppsDomain == "" || !strings.Contains(c.AppsDomain, ".") {
		errs = append(errs, errors.New("FOYR_APPS_DOMAIN must be set, e.g. apps.foyr.online"))
	}
	if v := getenv("FOYR_APP_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p <= 0 || p > 65535 {
			errs = append(errs, fmt.Errorf("FOYR_APP_PORT must be a port number, got %q", v))
		}
		c.AppPort = p
	}
	if v := getenv("FOYR_STATIC_ROUTES"); v != "" {
		m, err := ParseStatic(v)
		if err != nil {
			errs = append(errs, err)
		}
		c.StaticRoutes = m
	} else {
		if c.RoutesURL == "" {
			errs = append(errs, errors.New("FOYR_ROUTES_URL is not set (or set FOYR_STATIC_ROUTES for local development)"))
		}
		if len(c.Token) < 32 {
			errs = append(errs, errors.New("FOYR_INTERNAL_TOKEN must be at least 32 characters"))
		}
	}
	errs = append(errs, loadAuth(c, getenv)...)
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return c, nil
}

// loadAuth reads the login settings; nothing is checked while FOYR_SESSION_PUBLIC_KEY is unset.
func loadAuth(c *Config, getenv func(string) string) []error {
	raw := strings.TrimSpace(getenv("FOYR_SESSION_PUBLIC_KEY"))
	if raw == "" {
		return nil
	}
	var errs []error
	pub, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		errs = append(errs, errors.New("FOYR_SESSION_PUBLIC_KEY must be 32 bytes in base64"))
	} else {
		c.SessionPublicKey = pub
	}
	c.AuthURL = strings.TrimRight(strings.TrimSpace(getenv("FOYR_AUTH_URL")), "/")
	u, uerr := url.Parse(c.AuthURL)
	local := uerr == nil && u.Scheme == "http" && (u.Host == "localhost" || strings.HasPrefix(u.Host, "localhost:"))
	switch {
	case c.AuthURL == "":
		errs = append(errs, errors.New("FOYR_AUTH_URL is not set (needed once FOYR_SESSION_PUBLIC_KEY is)"))
	case uerr != nil || u.Host == "" || u.Path != "" || !(u.Scheme == "https" || local):
		errs = append(errs, errors.New("FOYR_AUTH_URL must be https://<host>, or http://localhost:<port> for development"))
	}
	if c.FoyrdURL == "" {
		errs = append(errs, errors.New("FOYR_FOYRD_URL is not set (needed once FOYR_SESSION_PUBLIC_KEY is)"))
	}
	if len(c.Token) < 32 && c.StaticRoutes != nil {
		errs = append(errs, errors.New("FOYR_INTERNAL_TOKEN must be at least 32 characters"))
	}
	return errs
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
