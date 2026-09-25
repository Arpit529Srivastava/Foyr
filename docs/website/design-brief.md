# Foyr Website — Design Brief

> Hand this file to the designer / design tool. It describes **what to build, how it should
> look, and how it should move**. Visual direction is inspired by rightfit.so (studied
> 2026-09-26), adapted to Foyr's product and brand. Do **not** copy Rightfit's logo, copy,
> photography or illustrations — match the *quality and feel*, not the assets.

---

## 1. What we're building

A marketing website + public docs for **Foyr**, hosted on **Vercel** at `foyr.online`.

**Foyr in one line:** Deploy the app you already wrote — with login, invites and roles built in.

**Primary goal:** collect **waitlist sign-ups** from developers.
**Secondary goal:** explain the product clearly enough that a developer "gets it" in 10 seconds,
and give them docs to go deeper.

**Audience:** solo developers and small teams who built something (often with Cursor / Claude
Code) and need to share it privately with teammates or clients — internal tools, client
dashboards, SaaS MVPs. Technical, skeptical of marketing fluff, impressed by craft.

**Pre-launch honesty rule:** Foyr has no customers yet. **No fake stats, fake testimonials, fake
logos or "trusted by" rows.** Where Rightfit uses social proof, we use product proof (real
mockups, code, comparisons).

---

## 2. The feel (what makes the reference site great)

Studied on rightfit.so — reproduce these qualities:

1. **Near-black, monochrome UI** with a single source of colour: rich, blurred photographic
   imagery *behind* crisp product mockups. The UI itself stays grey-scale.
2. **Product-as-hero.** The biggest thing above the fold is a realistic, dense, *interactive*
   mockup of the app inside a framed window — not an illustration.
3. **Quiet, confident typography.** Inter at weight ~510 with tight negative tracking on
   headings; muted grey body copy; lots of empty space.
4. **Hairline borders, not shadows.** Cards are `#191919` on `#000` with 1px `#2a2a2a` borders
   and small radii. Depth comes from contrast, not drop shadows.
5. **Calm motion.** Every section fades up 20px as it enters. Hover states are 150–300ms colour
   transitions. Nothing bounces, nothing spins. Smooth scrolling throughout.
6. **Small delightful details:** blurred sticky header, a text "chroma" shimmer on small labels,
   an infinite logo/marquee strip, draggable cards inside mockups, an FAQ accordion.

---

## 3. Tech stack

| Concern | Choice |
|---|---|
| Framework | **Next.js (App Router)**, deployed on **Vercel** |
| Styling | **Tailwind CSS v4** + **shadcn/ui** (Radix primitives) |
| Font | **Inter** variable (100–900) via `next/font` |
| Motion | **Motion** (`motion/react`, formerly Framer Motion) |
| Smooth scroll | CSS `scroll-behavior: smooth`; optional **Lenis** for inertial wheel scrolling |
| Drag in mockups | `@dnd-kit/core` (only for the roles/kanban mockup) |
| Docs | **Fumadocs** (Next.js-native, MDX) at `/docs`, same theme tokens |
| Waitlist | Server Action → **Resend** audience (or Postgres); double opt-in email |
| Analytics | **PostHog** (free tier) + Vercel Analytics |
| Icons | **Lucide** |
| Location in repo | `site/` in the monorepo |

---

## 4. Design tokens

Dark mode is the default and primary theme. Ship light mode tokens too (docs readers like it),
but design dark first.

### 4.1 Colour (dark — default)

```css
.dark {
  /* grey scale */
  --gray-100: #111;  --gray-200: #191919; --gray-300: #222;  --gray-400: #2a2a2a;
  --gray-500: #313131; --gray-600: #3a3a3a; --gray-700: #484848; --gray-800: #606060;
  --gray-900: #6e6e6e; --gray-1000: #7b7b7b; --gray-1100: #b5b5b5; --gray-1200: #eee;

  --background: #000;          --foreground: #eee;
  --card: #191919;             --card-foreground: #eee;
  --popover: #191919;
  --primary: #eee;             --primary-foreground: #000;   /* white buttons */
  --secondary: #222;           --secondary-foreground: #b5b5b5;
  --muted: #2a2a2a;            --muted-foreground: #7b7b7b;
  --border: #2a2a2a;           --input: #2a2a2a;             --ring: #b5b5b5;

  --shadow-border: 0 0 0 1px #ffffff14;
  --shadow-border-hover: 0 0 0 1px #ffffff21;
  --shadow-focus: 0 0 0 3px #ffffff1a;

  /* status colours — used ONLY inside product mockups (badges, logs) */
  --success: #4ade80; --warning: #fbbf24; --info: #22d3ee;
  --destructive: #e5484d; --iris: #818cf8; --lavender: #a78bfa;

  /* Foyr brand accent — used sparingly (focus rings, one CTA glow, link underline) */
  --foyr-amber: #f5a524;
}
```

### 4.2 Colour (light)

```css
:root {
  --background: #fcfcfc; --foreground: #000; --card: #fff; --primary: #000;
  --primary-foreground: #fff; --secondary: #f6f6f6; --muted: #f0f0f0;
  --muted-foreground: #646464; --border: #e8e8e8; --ring: #646464;
  --shadow-border: 0 0 0 1px #0000000f;
}
```

### 4.3 Typography

Font: **Inter**, fallback `ui-sans-serif, system-ui, -apple-system`.

| Role | Size / line-height | Weight | Tracking | Colour |
|---|---|---|---|---|
| Display (hero h1) | 60 / 72px (mobile 36 / 44) | 510 | -0.022em (≈ -1.3px) | `#fff` |
| Section h2 | 36 / 40px (mobile 28 / 34) | 510 | -0.012em | `#eee` |
| Feature h3 | 24–28px | 510 | -0.012em | `#eee` |
| Card title | 14–15px | 500–510 | normal | `#fff` |
| Lead paragraph | 18 / 28px | 400 | normal | `#a3a3a3` (≈ neutral-400) |
| Body / card copy | 14–16px | 400 | normal | `neutral-400` |
| Eyebrow label | 11–12px, UPPERCASE | 400–500 | 0.08em | `neutral-500`, in a 1px bordered pill |
| Code | 13–14px **Geist Mono** or JetBrains Mono | 400 | normal | `#eee` |

### 4.4 Shape & spacing

- **Radius scale:** 4 · 6 · 8 · 12 · 16 · 20px. Buttons 8px. Cards 8–12px. Mockup frames 12–16px.
  Image panels 16–20px. Base `--radius: 0.625rem`.
- **Container:** `max-width: 1152px`, centred, 24px side padding (16px on mobile).
- **Section spacing:** 128–160px vertical between sections on desktop, 80–96px on mobile.
- **Grid:** 4-column bento on desktop, 2 on tablet, 1 on mobile. 12–16px gaps.
- **Borders:** 1px `--border` everywhere; hover lifts border to `#ffffff21`.

---

## 5. Global components

### Header (sticky)
- `position: fixed`, height **57px**, background `rgba(0,0,0,0.8)` + `backdrop-filter: blur(12px)`,
  bottom border `1px rgba(255,255,255,0.08)`, `z-index` above everything.
- Left: Foyr logomark (see §8). Centre: **Product ▾ · Docs · Pricing · Changelog**. Right:
  secondary button **"Read the docs"**, primary button **"Join waitlist →"**.
- "Product ▾" opens a mega-menu (fade + 4px slide, 200ms): Deploy · Auth & invites · Roles ·
  Custom domains — each with a one-line description.
- Mobile: logo + hamburger → full-screen sheet menu.

### Buttons
- **Primary:** white bg `#eee`, black text, 8px radius, 40px tall (32px in header), 14px/500,
  trailing `→` that nudges 2px right on hover.
- **Secondary:** transparent / `#111` bg, `#eee` text, 1px `#2a2a2a` border, hover border `#484848`.
- Transitions: `color, background-color, border-color 150ms cubic-bezier(0.4,0,0.2,1)`.

### Eyebrow pill
Small bordered pill above sections: `FOR DEVELOPERS`, `HOW IT WORKS`, `[ FAQ ]`.

### Card
`#191919` bg, 1px border, 8–12px radius, 20px padding. Title top-left (white, 15px/510), body
bottom-left (neutral-400, 14px). Tall (≈210px) with empty middle space — the negative space is
intentional. On hover: border brightens and the small label above does the chroma shimmer (§7).

### Browser-window frame (for mockups)
Dark window chrome: three grey dots, a rounded URL pill with a lock icon (e.g.
`acme-crm.foyr.online`), reload icon. Mockup content inside. Frame sits on a blurred colour image.

### Footer
Four link columns (Product · Resources · Company · Connect), a one-line mission, "Made in India 🇮🇳",
© line, Terms · Privacy. Plus an **"Ask about Foyr on"** row with small ChatGPT / Claude /
Perplexity / Gemini icons that open each assistant with a pre-filled prompt about Foyr.

---

## 6. Page: Home (`/`) — section by section

### 6.1 Hero
- **Layout:** two columns. Left: h1. Right (aligned to baseline): lead paragraph + 2 buttons.
- **h1:** "Deploy the app you already built. Login included."
- **Lead:** "Push your repo. Foyr gives it a URL, a front door and a guest list — invite people by
  email, gate features by role. No auth code. No AWS console."
- **Buttons:** `Join waitlist →` (primary) · `Read the docs` (secondary).
- **Below:** full-width panel (16–20px radius) with a **tab bar** on top:
  `🚀 Deploy` · `🔑 Invite & roles` · `📜 Logs`. Clicking a tab cross-fades (300ms) the mockup
  inside. The panel background is a **blurred, warm photographic image** (see §8); the mockup
  window floats centred on it with a 1px light border.
  - *Deploy tab:* Foyr dashboard — sidebar (Apps, Deployments, Members, Domains, Settings), a
    deployment list with statuses (`Live` green, `Building` amber pulse, `Failed` red), and a
    right panel showing the live URL `acme-crm.foyr.online`, commit SHA, "Deployed 2m ago".
  - *Invite & roles tab:* members table (avatar, email, role dropdown Owner/Admin/Member/Viewer,
    status "Invited"/"Active"), an "Invite people" input with chips.
  - *Logs tab:* streaming build log in mono font, lines appearing one by one
    (`▸ Detected Next.js 15 · pnpm` → `▸ Building…` → `✓ Health check passed` → `✓ Live`).

### 6.2 "What you get" bento (8 cards, 4×2)
Headline: "Every app gets a front door." Sub: "Hosting, login and permissions — one push, no glue."

| Label pill | Card title | Body |
|---|---|---|
| Deploy | Push to deploy | Connect a GitHub repo. Every push builds and ships automatically. |
| Auth | Zero auth code | Magic-link login in front of your app. You never write a login page. |
| Private | Private by default | Only people you invite can open your app. Flip to public anytime. |
| Invites | Invite by email | Teammates and clients get a link, not a password. Access per app. |
| Roles | Four built-in roles | Owner, admin, member, viewer. Gate features with one function call. |
| Safe | Zero-downtime deploys | New versions go live only after health checks pass. Instant rollback. |
| Domains | Custom domains | Bring `app.yourco.com`. TLS certificates are automatic. |
| Region | Hosted in Mumbai | Low latency for India, on AWS. More regions later. |

### 6.3 "For developers" — three image cards
Eyebrow `FOR DEVELOPERS`. h2 left: "Ship it today. Share it safely." Paragraph right.
Three tall image cards (colourful blurred photo backgrounds), each with a small **white** UI card
floating in the centre, then title + description below the image:
1. **Your app, live** — mini card: URL + green "Live" dot + sparkline of requests.
2. **Who's inside** — mini card: 3 avatars with roles and "Invited 2h ago".
3. **Every deploy, tracked** — mini card: bar chart of deploys Mon–Fri with a tooltip "3 deploys".

### 6.4 How it works — sticky scroll story (01 / 02 / 03)
Left column (≈40%): numbered steps. Right column: a large browser-window mockup that bleeds off
the right edge of the viewport. As the user scrolls, the left text changes and the mockup swaps.
Each step has: number (`01`), h3, paragraph, then a thin divider and a sub-heading + detail.

1. **Connect your repo** — "Install the Foyr GitHub app on the repos you choose. Read-only, per
   repo." Mockup: repo picker list with language icons and "Connect" buttons.
2. **Push to deploy** — "We detect Next.js, build it, and ship it behind a health check."
   Mockup: kanban-style deployment board **Queued → Building → Live**, cards are draggable
   (dnd-kit) purely for delight.
3. **Invite your people** — "Add emails, pick a role, done. They log in with a magic link."
   Mockup: app access page with members + an invite email preview.

### 6.5 "The only code you write" — SDK snippet
Two columns. Left: h2 "Auth without the auth code." + short paragraph: "Foyr's proxy handles
login before requests reach your app. Read the user from a header — or use our 50-line SDK."
Right: code window with tabs **Next.js · Express · FastAPI**:

```ts
import { getUser, hasPermission } from "@foyr/auth";

export async function GET(req: Request) {
  const user = getUser(req);            // { id, email, role }
  if (!hasPermission(user, "reports:export")) {
    return new Response("Forbidden", { status: 403 });
  }
  return Response.json(await exportReports());
}
```
Syntax highlighting in muted greys with one accent colour. Copy button top-right.

### 6.6 Comparison (replaces testimonials)
h2: "Hosting and auth, finally in one place." A clean table, hairline rows:

| | Foyr | Vercel / Railway / Render | + Auth0 / Clerk / Cognito |
|---|---|---|---|
| Deploy from GitHub | ✓ | ✓ | — |
| Login in front of your app | ✓ built in | — | ✓ you integrate it |
| Invite people per app | ✓ | — | partial |
| Roles & permissions | ✓ | — | ✓ you build it |
| Auth code you write | **none** | — | lots |

Foyr column subtly highlighted (slightly lighter card background).

### 6.7 FAQ
Two columns: left eyebrow `[ FAQ ]`, h2 "Frequently asked questions", sub-copy. Right: accordion
(shadcn Accordion, 1px bordered rows, chevron rotates 180°, 200ms height animation).
Questions:
- What kind of apps can I deploy? *(Next.js first; Node & Python next; any Dockerfile works.)*
- Do I need to change my code to get login? *(No. The proxy handles it. The SDK is optional.)*
- Who can see my app? *(Only people you invite, unless you make it public.)*
- Where is my app hosted? *(AWS Mumbai, ap-south-1.)*
- How much will it cost? *(Free during early access. Simple pricing at launch.)*
- When can I use it? *(Join the waitlist — we're onboarding in small batches.)*

### 6.8 Final CTA
Full-bleed image band (≈520px tall), dark vignette on all edges fading into black. Big h1-size
text left: **"Every app deserves a front door."** Buttons: `Join waitlist →` · `Read the docs`.

### 6.9 Waitlist form (modal + inline)
Email input + "Join" button in a single bordered pill. States: idle, loading (spinner in button),
success ("You're on the list. Check your inbox."), error (inline, red text). Optional second step:
"What will you deploy?" (Internal tool / Client dashboard / SaaS MVP / Other).

---

## 7. Motion spec

| Pattern | Spec |
|---|---|
| **Section reveal** | Elements with `data-reveal` start `opacity:0; translateY(20px)`; animate to `1 / 0` when 15% in view, **600ms**, ease `cubic-bezier(0,0,0.2,1)`. Stagger children 60–80ms. Only once. |
| **Hero entrance** | h1 → lead → buttons → mockup, staggered 80ms; mockup additionally scales 0.98 → 1 over 1.2s. |
| **Hover (default)** | colour / background / border, **150–200ms**, `cubic-bezier(0.4,0,0.2,1)`. |
| **Card hover** | border `#2a2a2a → #ffffff21`, 300ms; eyebrow label does **chroma shimmer**. |
| **Chroma shimmer** | Text fills with a moving gradient (`background-clip:text`, gradient `#666 → #fff → #666`, 200% width, sweeps 0.8s once). |
| **Marquee** | Infinite horizontal scroll (`@keyframes marquee-x`), ~40s per loop, pauses on hover, edge fade masks left/right. Use it for **supported frameworks / tech logos** (Next.js, Node, Python, Docker, Postgres, GitHub, AWS) — not customer logos. |
| **Tabs (hero)** | Active pill slides under the tab (`layoutId`), content cross-fades 300ms. |
| **Mockup details** | "Building" badge pulses; log lines type in; counters tick up once when in view. |
| **Scroll** | `scroll-behavior: smooth`; Lenis (`lerp: 0.1`) for wheel inertia on desktop only. |
| **Reduced motion** | Respect `prefers-reduced-motion`: disable reveals, marquee, typing; keep instant state changes. |

Rule of thumb: **nothing faster than 150ms, nothing slower than 1.2s, nothing that bounces.**

---

## 8. Brand & imagery

- **Name:** Foyr — "the entrance of your app". Lowercase in the logotype: `foyr`.
- **Logomark idea:** a minimal geometric doorway / open-door glyph built from 4–6 rectangles on a
  grid (similar weight and pixel-grid feel to a monogram, but a *doorway*). White on black.
- **Imagery:** the only colour on the site. **Warm light through doorways and openings** —
  abstract, heavily blurred, grainy photography: amber/orange light, teal shadows, soft bokeh,
  light leaks. Think "light spilling through a half-open door at dusk". Use for: hero panel,
  3 feature cards, final CTA band. Generate or license; never use stock with people's faces.
- **Accent:** `--foyr-amber #f5a524`, used in < 2% of the UI (focus ring, one glow, active states).
- **Voice:** short, direct, developer-to-developer. No buzzwords ("revolutionary", "seamless",
  "leverage", "AI-powered"). Say what it does.

---

## 9. Other pages

| Route | Content |
|---|---|
| `/docs` | Fumadocs. Sidebar + content + "on this page" TOC. Same tokens and Inter. Start with: Introduction · Quickstart · How auth works · Roles & permissions · SDK reference · Architecture · FAQ. Search (⌘K). |
| `/pricing` | Placeholder: "Free during early access" + 3 tier cards greyed out "Coming soon" + waitlist CTA. |
| `/changelog` | Timeline of dated entries (MDX). First entry: "Foyr is being built". |
| `/privacy`, `/terms` | Simple long-form text pages. |
| `404` | "This door's locked." + button home. |

---

## 10. Responsive

- **≥1280:** as described; how-it-works mockups bleed off the right edge.
- **768–1279:** hero stacks (h1 on top, lead + buttons below, left-aligned); bento 2 columns;
  how-it-works becomes non-sticky, mockup under each step.
- **<768:** everything centred in hero (as reference does on mobile); h1 36px; buttons side by
  side at equal width; hero tabs become icon-only; bento 1 column; comparison table scrolls
  horizontally inside its card; header collapses to logo + hamburger.
- Minimum tap target 44px. No horizontal page scroll at 360px.

---

## 11. Quality bar

- Lighthouse ≥ 95 performance / 100 accessibility / 100 SEO on the home page.
- LCP < 2.0s on 4G: hero image as optimised AVIF/WebP via `next/image`, `priority`, blurred placeholder.
- All text contrast AA (muted grey on black must be ≥ `#8a8a8a` for body text).
- Keyboard: visible focus ring (`--shadow-focus` + amber outline), tabs & accordion fully keyboard operable.
- Metadata: title, description, OG image (dark, logo + tagline), Twitter card, favicon set, `sitemap.xml`, `robots.txt`.
- Never hardcode the domain; read `NEXT_PUBLIC_SITE_URL`.

---

## 12. Deliverables expected from design

1. Home page — desktop (1440) and mobile (390) frames, dark mode.
2. Header mega-menu, mobile menu, waitlist modal (all states).
3. The three hero mockups (Deploy / Invite & roles / Logs) and three how-it-works mockups at full fidelity.
4. Docs page template (sidebar, article, code block, callout, TOC) — dark and light.
5. Pricing placeholder, 404.
6. Logo mark + favicon + OG image.
7. Component sheet: buttons, pills, cards, inputs, accordion, tabs, code window, browser frame.
