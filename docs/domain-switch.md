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
- [x] Set `WAITLIST_FROM_EMAIL` in Vercel, e.g. `Foyr <hello@mail.foyr.online>`. This switches on the welcome email to people who join.
- [ ] Replace the hardcoded `FOUNDER_EMAIL` in `site/app/actions/waitlist.ts` with `WAITLIST_NOTIFY_EMAIL` only (or a `@foyr.online` address), and update its test. Needs a mailbox or forwarding for `@foyr.online` first. The same address is the reply-to of the welcome email.
- [x] Send a test sign-up and check both emails land in the inbox, not spam. Regenerate the screenshots in `docs/website/emails/`.

## Later, separate purchase
- [ ] Separate apps domain (`foyrapps.online` or cheaper, e.g. `.com`), plus the Public Suffix List entry (ADR-015). **Deadline: before the first outside user deploys an app** (planned Oct 24 to 30, docs/yc-plan.md).
- [ ] Until then `APPS_DOMAIN` = `apps.foyr.online`, founder's apps only (ADR-028). At the switch: buy the domain, add its DNS zone and wildcard certificate (`tofu apply`), change `APPS_DOMAIN` (`apps_domain` in `infra/variables.tf`, ADR-031), delegate its DNS to Route 53, redeploy, update the docs pages that mention `foyrapps.online`.
- [x] `authproxy_static_routes` bridge removed in step 8c (ADR-032).

## Platform subdomains (ADR-032)
- `api.foyr.online` is its own Route 53 zone (`api_domain` in `infra/variables.tf`), delegated with 4 NS records at GoDaddy. `auth.` and `app.` will follow the same pattern.
