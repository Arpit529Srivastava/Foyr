# foyr.online: marketing site + docs

Next.js 16 (App Router, Turbopack) · Tailwind CSS v4 · Motion · MDX docs · deployed on Vercel.
Built from the Foyr Claude Design canvas and [`docs/website/design-brief.md`](../docs/website/design-brief.md).

## Run it

```bash
npm install
cp .env.example .env.local   # add Resend keys to make the waitlist work
npm run dev
```

## Layout

| Path | What |
|---|---|
| `app/(marketing)/` | Home, pricing, changelog, privacy, terms (shared header + footer) |
| `app/docs/**/page.mdx` | Docs pages. Add a page here **and** in `lib/docs-nav.ts` (sidebar, pager, search, sitemap) |
| `app/actions/waitlist.ts` | Server action: adds emails to a Resend audience |
| `components/mockups/` | Product mockups **generated** from the design canvas by `scripts/dc2jsx.py`. Regenerate, don't hand-edit |
| `components/docs/` | Docs shell, ⌘K search, table of contents, MDX components (`Callout`, `CodeTabs`, `ProxyDiagram`) |
| `lib/code-themes.mjs` | Shiki themes (monochrome + amber) for code blocks |
| `app/opengraph-image.tsx`, `app/icon.svg`, `app/apple-icon.tsx` | Social card and icons |

## Regenerate a mockup

Read the board from the design canvas (e.g. `project/HeroDeploy.dc.html`), then:

```bash
python3 scripts/dc2jsx.py path/to/HeroDeploy.dc.html HeroDeployMockup components/mockups/hero-deploy.tsx
npx prettier --write components/mockups/hero-deploy.tsx
```

## Deploy (Vercel)

1. vercel.com → **Add New… → Project** → import `Arpit529Srivastava/Foyr` (private repos work through the Vercel GitHub app).
2. **Root Directory: `site`**. Framework preset: Next.js. Leave build settings as they are.
3. Add the environment variables from `.env.example`.
4. **Domains:** add `foyr.online` and `www.foyr.online`, then create the DNS records Vercel shows you at your registrar (GoDaddy).
