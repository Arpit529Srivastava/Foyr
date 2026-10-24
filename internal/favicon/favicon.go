// Package favicon serves the Foyr icon, the same one the site uses, and holds it for pages that inline it.
package favicon

import (
	_ "embed"
	"net/http"
	"net/url"
	"strings"
)

// SVG is the site's icon (site/app/icon.svg; a test keeps them equal), so a Foyr tab looks the same
// on foyr.online and in the product.
const SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32" shape-rendering="crispEdges"><rect width="32" height="32" rx="7" fill="#000"/><g fill="#fff" transform="translate(4 4) scale(1.5)"><rect x="2" y="1" width="12" height="2"/><rect x="2" y="3" width="2" height="12"/><rect x="12" y="3" width="2" height="12"/><rect x="5" y="4" width="4" height="11"/></g></svg>`

//go:embed favicon.png
var faviconPNG []byte

// Register serves the icon at /favicon.svg and /favicon.png, and at /favicon.ico, the path browsers
// ask for when a page names no icon (a PNG answers there). Pages on foyrd's hosts link /favicon.svg and
// /favicon.png; the PNG covers Safari.
func Register(mux *http.ServeMux) {
	serve := func(ctype string, body []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", ctype)
			w.Header().Set("Cache-Control", "public, max-age=86400")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			_, _ = w.Write(body)
		}
	}
	mux.HandleFunc("GET /favicon.svg", serve("image/svg+xml", []byte(SVG)))
	mux.HandleFunc("GET /favicon.png", serve("image/png", faviconPNG))
	mux.HandleFunc("GET /favicon.ico", serve("image/png", faviconPNG))
}

// DataLink is a <link> with the icon inside the page, for pages on customer hosts, where
// /favicon.svg belongs to the customer's app and must pass through untouched.
func DataLink() string {
	enc := strings.ReplaceAll(url.QueryEscape(SVG), "+", "%20")
	return `<link rel="icon" href="data:image/svg+xml,` + enc + `">`
}
