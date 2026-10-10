# Domain switch checklist

Things that were temporary until `foyr.online` was bought (ADR-021). The domain was bought on
2026-10-10 (ADR-022); tick items off as they are done.

## Website (Vercel)
- [x] Add `foyr.online` (and `www`) in Vercel → Settings → Domains; add the DNS records at GoDaddy.
- [x] Redirect `foyrai.vercel.app` to `https://foyr.online` in Vercel.
- [x] Set `NEXT_PUBLIC_SITE_URL=https://foyr.online` in Vercel and redeploy.
- [x] Change the `siteUrl` fallback in `site/lib/site.ts` back to `https://foyr.online`, and update `site/lib/site.test.ts` and `site/.env.example`.
- [x] Check search indexing switched on by itself (`isIndexable`): `/robots.txt` says `Allow: /`, no `noindex` meta. Run the `foyr-live-check` skill.

## Email (Resend)
- [x] Verify the sending subdomain `mail.foyr.online` in Resend (SPF, DKIM, DMARC records at GoDaddy) (ADR-001).
- [ ] Set `WAITLIST_FROM_EMAIL` in Vercel, e.g. `Foyr <hello@mail.foyr.online>`. This switches on the welcome email to people who join.
- [ ] Replace the hardcoded `FOUNDER_EMAIL` in `site/app/actions/waitlist.ts` with `WAITLIST_NOTIFY_EMAIL` only (or a `@foyr.online` address), and update its test. Needs a mailbox or forwarding for `@foyr.online` first. The same address is the reply-to of the welcome email.
- [ ] Send a test sign-up and check both emails land in the inbox, not spam. Regenerate the screenshots in `docs/website/emails/`.

## Later, separate purchase
- [ ] `foyrapps.online` for customer apps (Phase 0, before public launch), plus the Public Suffix List entry (ADR-015).
