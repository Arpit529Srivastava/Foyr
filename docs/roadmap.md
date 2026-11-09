# Roadmap

**Now racing for the YC deadline (Nov 2, 2026).** docs/yc-plan.md picks the items below that go into the
demo and lists what waits until after applying.

## Now: Website & docs (Vercel)
- [x] Marketing site built (`site/`): home, pricing, changelog, 404, privacy, terms
- [x] Public docs section: 10 pages, search, light/dark
- [x] Waitlist form (Resend audience)
- [x] CI for `site/` (lint, format, typecheck, build) and protected `main` (ADR-017)
- [x] Deploy to Vercel and point `foyr.online` at it (ADR-022)
- [x] Resend audience, founder notification and welcome email from `mail.foyr.online`
- [ ] Replace plain-language privacy/terms with reviewed versions before hosting customer apps

## Phase 0: Bare deploy pipeline (~3 weeks)
- [ ] OpenTofu: VPC, subnets, fck-nat, ALB, ECS cluster, ECR, RDS, CodeBuild project (ap-south-1)
- [ ] GitHub App: repo install, verified push webhooks, installation tokens (developer login moves to Phase 1, ADR-023)
- [x] Platform database, job queue, audit trail and delivery log; push → queued deployment (step 6a, ADR-025, ADR-026)
- [x] Build: repo Dockerfile or Next.js template → CodeBuild → ECR (step 6b, ADR-027; first live builds 2026-10-10)
- [x] Deploy: ECS service per deployment, Cloud Map discovery, health check (step 7, ADR-029; first live app 2026-10-10)
- [ ] authproxy as a plain router with proxy-level blue/green and rollback (step 8: routing code done, ADR-030; running in AWS pending)
- [ ] `<app>.apps.foyr.online` wildcard routing + reserved slugs for now (ADR-028); buy the separate apps domain before the first outside user
- **Done when:** `git push` → live URL, working and repeatable.

## Phase 1: Auth-as-a-service (~4 weeks)
- [ ] Developer login with GitHub (ADR-023)
- [x] Magic-link login and sessions: code, tests and local run done (ADR-035; Resend and Ed25519 tokens instead of SES and JWT); live in AWS pending
- [x] authproxy: session check, header stripping, identity headers (signed assertion later)
- [x] Per-app invites (founder page and dashboard Access tab, ADR-035)
- [ ] `@foyr/auth` SDK: `getUser()`
- [ ] Basic dashboard: apps list, deploy, CI-style deployment view (steps, timeline, logs, ADR-024), audit log
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
