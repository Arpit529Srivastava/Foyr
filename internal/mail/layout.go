package mail

import (
	"html"
	"strconv"
	"strings"
)

// Email-safe building blocks (tables, inline styles) with the same design tokens as the waitlist emails in
// site/lib/emails/layout.ts, so every Foyr email looks like one product.
const (
	colInk    = "#000000"
	colPanel  = "#111111"
	colCard   = "#191919"
	colLine   = "#2a2a2a"
	colFg     = "#eeeeee"
	colMuted  = "#a3a3a3"
	colSubtle = "#8a8a8a"
	colAmber  = "#f5a524"

	fontSans = "Inter, ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
	fontMono = "'Geist Mono', 'JetBrains Mono', ui-monospace, 'SF Mono', Menlo, monospace"
)

func esc(s string) string { return html.EscapeString(s) }

// mark is the Foyr doorway drawn with table cells, so it shows even when images are blocked.
func mark() string {
	const unit = 2
	type cell struct {
		width  int
		filled bool
	}
	rows := []struct {
		height int
		cells  []cell
	}{
		{2, []cell{{12, true}}},
		{1, []cell{{2, true}, {8, false}, {2, true}}},
		{11, []cell{{2, true}, {1, false}, {4, true}, {3, false}, {2, true}}},
	}
	var b strings.Builder
	b.WriteString(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" aria-hidden="true">`)
	for _, r := range rows {
		b.WriteString(`<tr><td style="padding:0;"><table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>`)
		for _, c := range r.cells {
			bg := ""
			if c.filled {
				bg = "background:" + colFg + ";"
			}
			w, h := strconv.Itoa(c.width*unit), strconv.Itoa(r.height*unit)
			b.WriteString(`<td width="` + w + `" height="` + h + `" style="width:` + w + `px;height:` + h +
				`px;padding:0;font-size:0;line-height:0;` + bg + `">&nbsp;</td>`)
		}
		b.WriteString(`</tr></table></td></tr>`)
	}
	b.WriteString(`</table>`)
	return b.String()
}

// banner is the site's doorway light. Without a public site (local development) there is no banner,
// rather than a broken image.
func banner(siteURL string) string {
	if siteURL == "" {
		return ""
	}
	return `<tr><td style="padding:0;font-size:0;line-height:0;"><img src="` + esc(siteURL) +
		`/email/banner.jpg" width="558" alt="" style="display:block;width:100%;max-width:558px;height:auto;border:0;border-radius:13px 13px 0 0;background:#06161a;" /></td></tr>`
}

func eyebrow(label string) string {
	return `<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
<td style="padding:0 8px 0 0;"><div style="width:6px;height:6px;border-radius:3px;background:` + colAmber + `;font-size:0;line-height:0;">&nbsp;</div></td>
<td style="font-family:` + fontMono + `;font-size:12px;line-height:16px;letter-spacing:0.04em;text-transform:uppercase;color:` + colMuted + `;">` + esc(label) + `</td>
</tr></table>`
}

// heading, paragraph and small take HTML the caller has already escaped.
func heading(htmlText string) string {
	return `<h1 style="margin:14px 0 12px;font-family:` + fontSans + `;font-size:26px;line-height:32px;font-weight:600;letter-spacing:-0.02em;color:#ffffff;">` + htmlText + `</h1>`
}

func paragraph(htmlText string) string {
	return `<p style="margin:0 0 16px;font-family:` + fontSans + `;font-size:15px;line-height:24px;color:` + colMuted + `;">` + htmlText + `</p>`
}

func small(htmlText string) string {
	return `<p style="margin:16px 0 0;font-family:` + fontSans + `;font-size:13px;line-height:20px;color:` + colSubtle + `;">` + htmlText + `</p>`
}

func strong(text string) string {
	return `<span style="color:` + colFg + `;font-weight:500;">` + esc(text) + `</span>`
}

func button(href, label string) string {
	return `<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:8px 0 4px;"><tr>
<td style="border-radius:8px;background:` + colFg + `;">
<a href="` + esc(href) + `" style="display:inline-block;padding:11px 18px;font-family:` + fontSans + `;font-size:14px;line-height:18px;font-weight:500;color:#000000;text-decoration:none;border-radius:8px;">` + esc(label) + ` &rarr;</a>
</td></tr></table>`
}

// details is a bordered panel of label and value rows.
func details(rows [][2]string) string {
	var cells strings.Builder
	for i, r := range rows {
		border := ""
		if i > 0 {
			border = "border-top:1px solid " + colLine + ";"
		}
		cells.WriteString(`<tr>
<td style="padding:12px 16px;` + border + `font-family:` + fontSans + `;font-size:13px;line-height:20px;color:` + colSubtle + `;width:84px;vertical-align:top;" width="84">` + esc(r[0]) + `</td>
<td style="padding:12px 16px;` + border + `font-family:` + fontMono + `;font-size:13px;line-height:20px;color:` + colFg + `;overflow-wrap:anywhere;word-break:break-word;">` + esc(r[1]) + `</td>
</tr>`)
	}
	return `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:8px 0 20px;border:1px solid ` + colLine +
		`;border-radius:10px;border-collapse:separate;table-layout:fixed;background:` + colPanel + `;">` + cells.String() + `</table>`
}

// page is the whole document: dark page, Foyr lockup, one card, small footer.
func page(title, preheader, body, footer, siteURL string) string {
	logo := `<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
<td style="padding:0 10px 0 0;vertical-align:middle;">` + mark() + `</td>
<td style="vertical-align:middle;font-family:` + fontSans + `;font-size:20px;line-height:24px;font-weight:600;letter-spacing:-0.04em;color:#ffffff;">foyr</td>
</tr></table>`
	if siteURL != "" {
		logo = `<a href="` + esc(siteURL) + `" style="text-decoration:none;">` + logo + `</a>`
	}
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<meta name="color-scheme" content="dark" />
<meta name="supported-color-schemes" content="dark" />
<title>` + esc(title) + `</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap" rel="stylesheet" />
</head>
<body style="margin:0;padding:0;background:` + colInk + `;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:` + colInk + `;">` + esc(preheader) + `</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:` + colInk + `;">
<tr><td align="center" style="padding:40px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;">
<tr><td style="padding:0 4px 24px;">` + logo + `</td></tr>
<tr><td style="padding:0;background:` + colCard + `;border:1px solid ` + colLine + `;border-radius:14px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
` + banner(siteURL) + `
<tr><td style="padding:28px 32px 32px;">
` + body + `
</td></tr>
</table>
</td></tr>
<tr><td style="padding:24px 4px 0;font-family:` + fontSans + `;font-size:12px;line-height:18px;color:` + colSubtle + `;">
` + footer + `
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`
}
