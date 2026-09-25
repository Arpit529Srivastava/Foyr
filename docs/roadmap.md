# Roadmap

## Now: Website & docs (Vercel)
- [x] Marketing site built (`site/`): home, pricing, changelog, 404, privacy, terms
- [x] Public docs section: 10 pages, search, light/dark
- [x] Waitlist form (Resend audience)
- [ ] Deploy to Vercel and point `foyr.online` at it
- [ ] Resend account + audience; set `RESEND_API_KEY` / `RESEND_AUDIENCE_ID`
- [ ] Replace plain-language privacy/terms with reviewed versions before hosting customer apps

## Phase 0: Bare deploy pipeline (~3 weeks)
- [ ] OpenTofu: VPC, subnets, fck-nat, ALB, ECS cluster, ECR, RDS, CodeBuild project (ap-south-1)
- [ ] GitHub App: developer login, repo install, push webhooks
- [ ] Build: repo Dockerfile or Next.js template → CodeBuild → ECR
- [ ] Deploy: ECS service per deployment, Cloud Map discovery, health check
- [ ] authproxy as a plain router with proxy-level blue/green and rollback
- [ ] Buy `APPS_DOMAIN` (foyrapps.online); `<app>.foyrapps.online` wildcard routing + reserved slugs
- **Done when:** `git push` → live URL, working and repeatable.

## Phase 1: Auth-as-a-service (~4 weeks)
- [ ] Magic-link login (SES) and sessions (ES256 JWT)
- [ ] authproxy: session check, header stripping, identity headers, signed assertion
- [ ] Per-app invites
- [ ] `@foyr/auth` SDK: `getUser()`
- [ ] Basic dashboard: apps list, deploy, status, build logs
- **Done when:** a deployed app has working login with zero auth code written by the user.

## Phase 2: Permissions (~4 weeks)
- [ ] Roles: owner / admin / member / viewer
- [ ] `hasPermission()` + optional `foyr.json`
- [ ] Role assignment UI
- [ ] Scale-to-zero (sleep / wake in authproxy)
- **Done when:** an app can gate a feature by role using only the SDK.

## Phase 3: Polish for first external users (~3 weeks)
- [ ] Custom domains + automatic TLS
- [ ] Logs & metrics viewer
- [ ] Tenant Postgres (shared Aurora, DB per app)
- [ ] Stripe billing (simple flat tiers)
- [ ] Onboarding polish
- **Done when:** a stranger can sign up, connect a repo, deploy and invite a teammate without help.
