// Email-safe building blocks (tables, inline styles) using the site's design tokens.
export const tokens = {
  ink: "#000000",
  panel: "#111111",
  card: "#191919",
  line: "#2a2a2a",
  fg: "#eeeeee",
  dim: "#b5b5b5",
  muted: "#a3a3a3",
  subtle: "#8a8a8a",
  amber: "#f5a524",
};

const font = "Inter, ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif";
const mono = "'Geist Mono', 'JetBrains Mono', ui-monospace, 'SF Mono', Menlo, monospace";

export type Email = { subject: string; html: string; text: string };

export function escapeHtml(value: string): string {
  return value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}

// The Foyr doorway mark drawn with table cells, so it shows even when images are blocked.
function mark(unit = 2): string {
  const rows: [number, [number, boolean][]][] = [
    [2, [[12, true]]],
    [
      1,
      [
        [2, true],
        [8, false],
        [2, true],
      ],
    ],
    [
      11,
      [
        [2, true],
        [1, false],
        [4, true],
        [3, false],
        [2, true],
      ],
    ],
  ];
  const row = ([height, cells]: [number, [number, boolean][]]) =>
    `<tr><td style="padding:0;"><table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>${cells
      .map(
        ([width, filled]) =>
          `<td width="${width * unit}" height="${height * unit}" style="width:${width * unit}px;height:${height * unit}px;padding:0;font-size:0;line-height:0;${filled ? `background:${tokens.fg};` : ""}">&nbsp;</td>`,
      )
      .join("")}</tr></table></td></tr>`;
  return `<table role="presentation" cellpadding="0" cellspacing="0" border="0" aria-hidden="true">${rows.map(row).join("")}</table>`;
}

export function eyebrow(label: string): string {
  return `<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
<td style="padding:0 8px 0 0;"><div style="width:6px;height:6px;border-radius:3px;background:${tokens.amber};font-size:0;line-height:0;">&nbsp;</div></td>
<td style="font-family:${mono};font-size:12px;line-height:16px;letter-spacing:0.04em;text-transform:uppercase;color:${tokens.muted};">${escapeHtml(label)}</td>
</tr></table>`;
}

export function heading(html: string): string {
  return `<h1 style="margin:16px 0 12px;font-family:${font};font-size:26px;line-height:32px;font-weight:600;letter-spacing:-0.02em;color:#ffffff;">${html}</h1>`;
}

export function paragraph(html: string): string {
  return `<p style="margin:0 0 16px;font-family:${font};font-size:15px;line-height:24px;color:${tokens.muted};">${html}</p>`;
}

export function strong(text: string): string {
  return `<span style="color:${tokens.fg};font-weight:500;">${escapeHtml(text)}</span>`;
}

export function button(href: string, label: string): string {
  return `<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:8px 0 4px;"><tr>
<td style="border-radius:8px;background:${tokens.fg};">
<a href="${escapeHtml(href)}" style="display:inline-block;padding:11px 18px;font-family:${font};font-size:14px;line-height:18px;font-weight:500;color:#000000;text-decoration:none;border-radius:8px;">${escapeHtml(label)} &rarr;</a>
</td></tr></table>`;
}

/** A bordered panel of label/value rows, like the dashboard mockups on the site. */
export function details(rows: [string, string][]): string {
  const cells = rows
    .map(
      ([label, value], i) => `<tr>
<td style="padding:12px 16px;${i ? `border-top:1px solid ${tokens.line};` : ""}font-family:${font};font-size:13px;line-height:20px;color:${tokens.subtle};width:84px;vertical-align:top;" width="84">${escapeHtml(label)}</td>
<td style="padding:12px 16px;${i ? `border-top:1px solid ${tokens.line};` : ""}font-family:${mono};font-size:13px;line-height:20px;color:${tokens.fg};overflow-wrap:anywhere;word-break:break-word;">${escapeHtml(value)}</td>
</tr>`,
    )
    .join("");
  return `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:8px 0 20px;border:1px solid ${tokens.line};border-radius:10px;border-collapse:separate;table-layout:fixed;background:${tokens.panel};">${cells}</table>`;
}

/** Title plus one-line description rows with an amber tick. */
export function features(items: [string, string][]): string {
  const rows = items
    .map(
      ([title, text], i) => `<tr>
<td style="padding:14px 0 14px 16px;${i ? `border-top:1px solid ${tokens.line};` : ""}width:20px;vertical-align:top;font-family:${font};font-size:14px;line-height:20px;color:${tokens.amber};">&#10003;</td>
<td style="padding:14px 16px 14px 8px;${i ? `border-top:1px solid ${tokens.line};` : ""}font-family:${font};font-size:14px;line-height:20px;">
<span style="color:${tokens.fg};font-weight:500;">${escapeHtml(title)}</span><br />
<span style="color:${tokens.muted};">${escapeHtml(text)}</span>
</td></tr>`,
    )
    .join("");
  return `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:8px 0 24px;border:1px solid ${tokens.line};border-radius:10px;border-collapse:separate;background:${tokens.panel};">${rows}</table>`;
}

/** Full email document: dark page, Foyr lockup, one card, small footer. */
export function layout({
  title,
  preheader,
  body,
  footer,
  siteUrl,
}: {
  title: string;
  preheader: string;
  body: string;
  footer: string;
  siteUrl: string;
}): string {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<meta name="color-scheme" content="dark" />
<meta name="supported-color-schemes" content="dark" />
<title>${escapeHtml(title)}</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap" rel="stylesheet" />
</head>
<body style="margin:0;padding:0;background:${tokens.ink};">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:${tokens.ink};">${escapeHtml(preheader)}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:${tokens.ink};">
<tr><td align="center" style="padding:40px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;">
<tr><td style="padding:0 4px 24px;">
<a href="${escapeHtml(siteUrl)}" style="text-decoration:none;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
<td style="padding:0 10px 0 0;vertical-align:middle;">${mark()}</td>
<td style="vertical-align:middle;font-family:${font};font-size:20px;line-height:24px;font-weight:600;letter-spacing:-0.04em;color:#ffffff;">foyr</td>
</tr></table>
</a>
</td></tr>
<tr><td style="padding:32px;background:${tokens.card};border:1px solid ${tokens.line};border-radius:14px;">
${body}
</td></tr>
<tr><td style="padding:24px 4px 0;font-family:${font};font-size:12px;line-height:18px;color:${tokens.subtle};">
${footer}
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`;
}
