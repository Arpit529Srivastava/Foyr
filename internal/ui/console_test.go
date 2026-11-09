package ui

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestAssetsServeEveryFontTheStylesheetNames(t *testing.T) {
	mux := http.NewServeMux()
	RegisterAssets(mux)
	urls := regexp.MustCompile(`url\((/assets/[^)]+)\)`).FindAllStringSubmatch(string(ConsoleHead("n")), -1)
	if len(urls) != len(fonts) {
		t.Fatalf("stylesheet names %d fonts, want %d", len(urls), len(fonts))
	}
	for _, u := range urls {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, u[1], nil))
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "font/woff2" || w.Body.Len() < 1000 {
			t.Errorf("%s: status %d, type %q, %d bytes", u[1], w.Code, w.Header().Get("Content-Type"), w.Body.Len())
		}
		if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "public") || !strings.Contains(cc, "max-age=") {
			t.Errorf("%s: Cache-Control %q; fonts should be cacheable", u[1], cc)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing nosniff", u[1])
		}
	}
}

func TestSecureHeadersAllowOurOwnFontsOnly(t *testing.T) {
	w := httptest.NewRecorder()
	SecureHeaders(w)
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "font-src 'self';") {
		t.Fatalf("CSP does not allow the console fonts: %s", csp)
	}
}

func TestThemeScriptCarriesThePageNonce(t *testing.T) {
	head := string(ConsoleHead("abc123"))
	if !strings.Contains(head, `<script nonce="abc123">`) || !strings.Contains(head, "foyr_theme") {
		t.Fatalf("theme script is missing its nonce: %.300s", head)
	}
	if strings.Contains(string(ConsoleHead("")), "<script") {
		t.Fatal("an empty nonce must leave the script out, or app login answers stop being identical")
	}
	if !strings.Contains(string(ThemeButton()), "data-theme-toggle") {
		t.Fatal("the theme button does not carry the attribute the script listens for")
	}
}

// Both themes live in the tokens; phones need 16px inputs (Safari zooms on smaller) and tables that stack.
func TestStylesheetDefinesThemesAndPhoneRules(t *testing.T) {
	for _, want := range []string{"--bg:light-dark(", "--text:light-dark(", ":root[data-theme=light]{color-scheme:light}", "font-size:16px", ".rows td:has(form,button)"} {
		if !strings.Contains(consoleCSS, want) {
			t.Errorf("console.css is missing %q", want)
		}
	}
}
