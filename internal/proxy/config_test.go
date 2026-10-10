package proxy

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	good := map[string]string{"FOYR_APPS_DOMAIN": "Apps.Foyr.Online", "FOYR_ROUTES_URL": "http://foyrd:9090/internal/routes",
		"FOYR_INTERNAL_TOKEN": strings.Repeat("t", 32)}
	c, err := Load(env(good))
	if err != nil || c.AppsDomain != "apps.foyr.online" || c.Addr != ":8081" || c.AppPort != 3000 || c.Namespace != "apps.foyr.internal" {
		t.Fatalf("c=%+v err=%v", c, err)
	}
	dev, err := Load(env(map[string]string{"FOYR_APPS_DOMAIN": "localhost.test", "FOYR_STATIC_ROUTES": "foyr-hello=localhost:3000"}))
	if err != nil || dev.StaticRoutes["foyr-hello"] != "localhost:3000" {
		t.Fatalf("dev=%+v err=%v", dev, err)
	}
	_, err = Load(env(map[string]string{"FOYR_INTERNAL_TOKEN": "short", "FOYR_APP_PORT": "nope"}))
	for _, want := range []string{"FOYR_APPS_DOMAIN", "FOYR_ROUTES_URL", "FOYR_INTERNAL_TOKEN", "FOYR_APP_PORT"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error %v does not mention %s", err, want)
		}
	}
}
