---
name: foyr-live-check
description: Use after a Foyr site deploy to verify the live site (routes, canonical, robots, sitemap, 404).
---

# Live site check

Base URL is the current `NEXT_PUBLIC_SITE_URL` (today https://foyrai.vercel.app, later https://foyr.online).

1. Routes return 200: `/`, `/pricing`, `/changelog`, `/privacy`, `/terms`, `/docs` and each docs page. Use `curl -s -o /dev/null -w "%{http_code}"`.
2. `/does-not-exist` returns 404 and shows "This door's locked."
3. Home page: canonical, `og:url`, `og:image` all use the base URL; `/sitemap.xml` entries use it too.
4. `/robots.txt`: `Disallow: /` on any host except foyr.online, `Allow: /` on foyr.online. Meta robots noindex only off foyr.online.
5. Waitlist: submit a test email, confirm it appears in the Resend audience, then remove it.
6. Report failures with the exact URL and status.
