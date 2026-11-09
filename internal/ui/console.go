package ui

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed console.css
var consoleCSS string

//go:embed fonts/*.woff2
var fontFiles embed.FS

// fonts are the typefaces console.css names, the same two foyr.online uses. Pages can only load them from
// foyrd, because SecureHeaders allows fonts from 'self' alone.
var fonts = []string{"inter-latin-wght.woff2", "geist-mono-latin-wght.woff2"}

// themeScript applies the saved theme before first paint and runs every theme button. Dark is the default,
// as on foyr.online, so only "light" changes anything.
const themeScript = `(()=>{const d=document.documentElement,k="foyr_theme";try{if(localStorage.getItem(k)==="light")d.dataset.theme="light"}catch(e){}` +
	`document.addEventListener("click",e=>{if(!e.target.closest("[data-theme-toggle]"))return;const l=d.dataset.theme!=="light";if(l)d.dataset.theme="light";else delete d.dataset.theme;try{localStorage.setItem(k,l?"light":"dark")}catch(e){}})})()`

// ConsoleHead returns what every console page puts in its <head>: the font preloads, the theme script (allowed
// by the page's nonce) and the shared stylesheet. An empty nonce leaves the script out: magic-link
// sign-in needs that, because its answers must be byte-identical whether or not an email has an account, so it stays dark.
func ConsoleHead(nonce string) template.HTML {
	out := ""
	for _, f := range fonts {
		out += `<link rel="preload" as="font" type="font/woff2" crossorigin href="/assets/` + f + `">`
	}
	if nonce != "" {
		out += `<script nonce="` + template.HTMLEscapeString(nonce) + `">` + themeScript + "</script>"
	}
	return template.HTML(out + "<style>" + consoleCSS + "</style>") // #nosec G203 -- embedded at build time; the nonce is escaped
}

// ThemeButton is the light and dark switch in the console's sidebar.
func ThemeButton() template.HTML {
	return `<button type="button" class="theme-btn" data-theme-toggle title="Switch light and dark" aria-label="Switch light and dark theme">` +
		`<svg class="sun" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>` +
		`<svg class="moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5Z"/></svg></button>`
}

// RegisterAssets serves the console fonts under /assets/. The files are versioned by the binary's release, and
// a month of caching keeps the no-store pages from fetching them every time.
func RegisterAssets(mux *http.ServeMux) {
	for _, name := range fonts {
		body, err := fontFiles.ReadFile("fonts/" + name)
		if err != nil {
			panic(err) // the file list above and the embed disagree: a build mistake
		}
		mux.HandleFunc("GET /assets/"+name, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "font/woff2")
			w.Header().Set("Cache-Control", "public, max-age=2592000")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			_, _ = w.Write(body)
		})
	}
}
