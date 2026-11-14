package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Arpit529Srivastava/Foyr/internal/authn"
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

func TestLoadAuthSettings(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	pub := base64.StdEncoding.EncodeToString(make([]byte, 32))
	token := strings.Repeat("t", 32)
	akey, apub := authn.NewAssertionKeyPair()
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p384der, _ := x509.MarshalPKCS8PrivateKey(p384)
	good := func() map[string]string {
		return map[string]string{
			"FOYR_APPS_DOMAIN": "apps.foyr.online", "FOYR_INTERNAL_TOKEN": token,
			"FOYR_FOYRD_URL": "http://foyrd.platform.foyr.internal:9090/", "FOYR_AUTH_URL": "https://auth.foyr.online/",
			"FOYR_SESSION_PUBLIC_KEY": pub, "FOYR_ASSERTION_KEY": akey,
		}
	}

	c, err := Load(env(good()))
	if err != nil || !c.AuthOn() || c.AuthURL != "https://auth.foyr.online" ||
		c.FoyrdURL != "http://foyrd.platform.foyr.internal:9090" ||
		c.RoutesURL != "http://foyrd.platform.foyr.internal:9090/internal/routes" || len(c.SessionPublicKey) != 32 || c.AssertionKey == nil {
		t.Fatalf("c=%+v err=%v", c, err)
	}

	// An explicit routes URL wins over the derived one.
	e := good()
	e["FOYR_ROUTES_URL"] = "http://elsewhere/internal/routes"
	if c, err := Load(env(e)); err != nil || c.RoutesURL != "http://elsewhere/internal/routes" {
		t.Fatalf("explicit routes url: %+v, %v", c, err)
	}

	// No public key: auth is off and nothing else is required, as before.
	off := map[string]string{"FOYR_APPS_DOMAIN": "apps.foyr.online", "FOYR_ROUTES_URL": "http://foyrd:9090/internal/routes", "FOYR_INTERNAL_TOKEN": token}
	if c, err := Load(env(off)); err != nil || c.AuthOn() {
		t.Fatalf("auth off: %+v, %v", c, err)
	}

	// Local development: static routes still need the token and foyrd's URL once auth is on.
	dev := good()
	delete(dev, "FOYR_INTERNAL_TOKEN")
	dev["FOYR_STATIC_ROUTES"] = "crm=localhost:3000"
	if _, err := Load(env(dev)); err == nil || !strings.Contains(err.Error(), "FOYR_INTERNAL_TOKEN") {
		t.Errorf("static routes with auth and no token: %v", err)
	}

	for name, edit := range map[string]func(map[string]string){
		"bad key":          func(m map[string]string) { m["FOYR_SESSION_PUBLIC_KEY"] = "nope" },
		"no auth url":      func(m map[string]string) { delete(m, "FOYR_AUTH_URL") },
		"http auth":        func(m map[string]string) { m["FOYR_AUTH_URL"] = "http://auth.foyr.online" },
		"no foyrd url":     func(m map[string]string) { delete(m, "FOYR_FOYRD_URL") },
		"no assertion key": func(m map[string]string) { delete(m, "FOYR_ASSERTION_KEY") },
		"malformed key":    func(m map[string]string) { m["FOYR_ASSERTION_KEY"] = "not-a-key-SECRETVALUE" },
		"p384 key":         func(m map[string]string) { m["FOYR_ASSERTION_KEY"] = base64.StdEncoding.EncodeToString(p384der) },
		"public key":       func(m map[string]string) { m["FOYR_ASSERTION_KEY"] = apub },
	} {
		m := good()
		edit(m)
		want := map[string]string{"bad key": "FOYR_SESSION_PUBLIC_KEY", "no auth url": "FOYR_AUTH_URL", "http auth": "FOYR_AUTH_URL", "no foyrd url": "FOYR_FOYRD_URL",
			"no assertion key": "FOYR_ASSERTION_KEY is not set", "malformed key": "FOYR_ASSERTION_KEY", "p384 key": "FOYR_ASSERTION_KEY", "public key": "FOYR_ASSERTION_KEY"}[name]
		_, err := Load(env(m))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want it to mention %s", name, err, want)
			continue
		}
		if v := m["FOYR_ASSERTION_KEY"]; v != "" && strings.Contains(err.Error(), v) {
			t.Errorf("%s: the error echoes the key value", name)
		}
	}
}
