package proxy

import (
	"errors"
	"fmt"
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
}

// Load reads the settings through getenv and reports every problem at once.
func Load(getenv func(string) string) (*Config, error) {
	c := &Config{
		Addr:       orDefault(getenv("FOYR_ADDR"), ":8081"),
		AppsDomain: strings.ToLower(strings.TrimSpace(getenv("FOYR_APPS_DOMAIN"))),
		RoutesURL:  getenv("FOYR_ROUTES_URL"),
		Token:      getenv("FOYR_INTERNAL_TOKEN"),
		Namespace:  orDefault(getenv("FOYR_APPS_NAMESPACE"), "apps.foyr.internal"),
		AppPort:    3000,
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
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return c, nil
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
