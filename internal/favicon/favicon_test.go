package favicon

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func serve(path string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	Register(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestFaviconRoutes(t *testing.T) {
	cases := []struct{ path, ctype string }{
		{"/favicon.svg", "image/svg+xml"},
		{"/favicon.png", "image/png"},
		{"/favicon.ico", "image/png"}, // browsers ask for this path by default; a PNG works there
	}
	for _, c := range cases {
		w := serve(c.path)
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != c.ctype {
			t.Errorf("%s: status %d, Content-Type %q, want 200 %q", c.path, w.Code, w.Header().Get("Content-Type"), c.ctype)
		}
		if w.Body.Len() < 100 {
			t.Errorf("%s: body is only %d bytes", c.path, w.Body.Len())
		}
		if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "public") || !strings.Contains(cc, "max-age=") {
			t.Errorf("%s: Cache-Control %q; icons should be cacheable", c.path, cc)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing nosniff", c.path)
		}
	}
	if !bytes.HasPrefix(serve("/favicon.png").Body.Bytes(), []byte("\x89PNG\r\n\x1a\n")) {
		t.Error("favicon.png is not a PNG")
	}
	if !strings.HasPrefix(serve("/favicon.svg").Body.String(), "<svg") {
		t.Error("favicon.svg is not an SVG")
	}
}

func TestFaviconOnlyAnswersGET(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/favicon.svg", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d, want 405", w.Code)
	}
}

// The icon is the site's icon, so a Foyr tab looks the same on foyr.online and in the product.
func TestFaviconIsTheSitesIcon(t *testing.T) {
	site, err := os.ReadFile("../../site/app/icon.svg")
	if err != nil {
		t.Skip("site/app/icon.svg not found")
	}
	if strings.TrimSpace(string(site)) != strings.TrimSpace(SVG) {
		t.Error("SVG differs from site/app/icon.svg; keep them the same")
	}
}

func TestDataLinkNeedsNoRoute(t *testing.T) {
	// Pages on customer hosts can't rely on /favicon.svg: that path belongs to the customer's app.
	l := DataLink()
	const pre = `<link rel="icon" href="`
	if !strings.HasPrefix(l, pre+"data:image/svg+xml,") || !strings.HasSuffix(l, `">`) {
		t.Fatalf("data favicon link malformed: %.120s", l)
	}
	href := strings.TrimSuffix(strings.TrimPrefix(l, pre), `">`)
	if strings.ContainsAny(href, "<>\" ") || strings.Contains(href, "/favicon") {
		t.Errorf("href is not a self-contained data URI: %.120s", href)
	}
}
