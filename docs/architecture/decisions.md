# Architecture Decision Records

Short records of the key decisions made before Phase 0, and why. Newest decisions go at
the bottom. To change a decision, add a new ADR that supersedes the old one.

---

## ADR-001: Name and domain: Foyr on `foyr.online`
**Date:** 2026-09-26 · **Status:** Accepted

**Decision:** The product is called **Foyr**; the platform domain is `foyr.online`.
(Customer app hostnames revised by [ADR-015](#adr-015-customer-apps-on-a-separate-domain).)

**Why:** Short and brandable, suggests "the entrance to your app". `foyr.online` was the
cheapest available option for the first year.

**Consequences:**
- `.online` has a weaker email reputation, so magic-link email must use a dedicated sending
  subdomain (`mail.foyr.online`) with SPF, DKIM and DMARC from day one.
- The domain must never be hardcoded; everything reads `PLATFORM_DOMAIN` so we can move later.
- An unrelated company (Foyr, interior-design software) owns `foyr.com` / `foyr.in`. Do a
  trademark check before any significant brand spend.

## ADR-002: Go for the control plane and proxy
**Date:** 2026-09-26 · **Status:** Accepted

**Decision:** foyrd and authproxy are written in **Go**. The dashboard is Next.js /
TypeScript. SDKs are written per target language.

**Why:** It's the founder's strongest language. Infra tooling is Go-native (Docker,
Kubernetes, Caddy, Railpack). `net/http/httputil.ReverseProxy` is ideal for the auth proxy.
Goroutines suit orchestration work. Single static binaries give small, fast-starting images.
AWS SDK for Go v2 is first-class.

**Consequences:** Two languages. The API is described in OpenAPI and the TypeScript client is
generated from it.

## ADR-003: OpenTofu for infrastructure as code
**Date:** 2026-09-26 · **Status:** Accepted

**Decision:** Static infrastructure is managed with **OpenTofu**, with state in S3 (native S3 locking).

**Why:** Free and fully open source (MPL), with no licensing ambiguity for a hosting company.
Same language and providers as Terraform.

**Consequences:** Per-app resources are *not* in OpenTofu; foyrd creates them at runtime.

## ADR-004: Monorepo
**Date:** 2026-09-26 · **Status:** Accepted

**Decision:** One repository for Go services, dashboard, marketing site, SDKs, infra and docs.

**Why:** Solo founder; cross-cutting changes are constant early on; one history, one CI.

**Consequences:** The public SDK is published to npm from `sdk/js/`; it can be split out later.

## ADR-005: Three deployables, not microservices
**Date:** 2026-09-26 · **Status:** Accepted

**Decision:** Run **web**, **foyrd** (API + auth + deployer + jobs in one binary) and
**authproxy**. No message broker, no service mesh.

**Why:** Scaling comes from horizontal replicas, not from service count. The high-traffic
path (tenant apps + proxy) is already isolated and autoscaled. authproxy is split out because
it is on the data path and must survive foyrd outages.

**When to split further:** a component needs very different resources, a bug in one part
keeps taking down another, or a second team needs independent deploys.

## ADR-006: Auth via proxy, not an injected SDK
**Date:** 2026-09-26 · **Status:** Accepted · **Supersedes:** original spec §4.2 (build-time SDK injection)

**Decision:** A Foyr-run **auth proxy** sits in front of every app, enforces login and
per-app access, and forwards signed identity headers. The SDK only reads and verifies those headers.

**Why:** True zero-code auth for any framework; no fragile build-time injection; apps that
never call the SDK are still protected; SDKs in new languages are trivial. The proxy also
enables proxy-level blue/green and scale-to-zero.

**Alternatives rejected:** build-time SDK injection (fragile, JS-only, unprotected by
default); ALB `authenticate-oidc` (limited login UX, per-ALB rule quotas, hard to pass roles).

## ADR-007: Builds: own Next.js template first, Railpack later
**Date:** 2026-09-26 · **Status:** Accepted · **Supersedes:** original spec's Nixpacks choice

**Decision:** Phase 0 builds use the repo's own `Dockerfile` if present, otherwise a
Foyr-maintained Next.js Dockerfile template. Adopt **Railpack** when a second framework is added.

**Why:** Phase 0 supports Next.js only; one template is simpler to debug than an external
builder. Nixpacks is in maintenance mode; Railpack is its successor.

**Consequences:** The build step is an interface (repo in, image out) so the builder can be swapped.

## ADR-008: Builds run on AWS CodeBuild
**Date:** 2026-09-26 · **Status:** Accepted

**Decision:** Each deploy runs as a CodeBuild job (`general1.medium`) that pushes to ECR.

**Why:** Builds execute untrusted code and need isolation; Fargate cannot run Docker; no idle cost.
~$0.01/min → roughly 2–5¢ per deploy.

**Consequences:** ECR lifecycle policy keeps the last 5 images per app; use S3 dependency cache.

## ADR-009: Scale-to-zero designed now, built in Phase 2–3
**Date:** 2026-09-26 · **Status:** Accepted

**Decision:** Phase 0–1 apps run one small always-on task (0.25 vCPU / 0.5 GB). The proxy and
data model are designed for sleep/wake, implemented in Phase 2–3.

**Why:** Keeps Phase 0 focused on proving deploys; cost of always-on test apps is small.

## ADR-010: GitHub App instead of OAuth App
**Date:** 2026-09-26 · **Status:** Accepted

**Why:** Per-repo, read-only install; built-in push webhooks; short-lived installation tokens
instead of stored user tokens.

## ADR-011: fck-nat instead of NAT Gateway during build phase
**Date:** 2026-09-26 · **Status:** Accepted

**Why:** ~$4/mo vs ~$35–50/mo. Move to a managed NAT Gateway once there are paying users.

## ADR-012: One shared Aurora cluster for tenant databases
**Date:** 2026-09-26 · **Status:** Accepted · **Supersedes:** original spec §4.6 (cluster per app)

**Decision:** One Aurora Serverless v2 cluster with a separate database and login per app.

**Why:** Roughly 20× cheaper than a cluster per app, still credential-isolated. Dedicated
clusters can be a paid-plan feature.

## ADR-013: Region ap-south-1 (Mumbai)
**Date:** 2026-09-26 · **Status:** Accepted

**Why:** Founder and early users are expected to be in India; lower latency for them.
All required services (Fargate, CodeBuild, Aurora Serverless v2, SES, ACM) are available.

## ADR-014: Marketing site and docs on Vercel
**Date:** 2026-09-26 · **Status:** Accepted

**Decision:** The public website and documentation live in `site/` and are hosted on Vercel,
separately from the AWS product platform.

**Why:** Fast to ship, free tier, and keeps marketing uptime independent of the platform.

## ADR-015: Customer apps on a separate domain
**Date:** 2026-09-26 · **Status:** Accepted · **Amends:** ADR-001, ADR-014

**Decision:**
- `foyr.online` → marketing site + docs (Vercel).
- `app.foyr.online` → dashboard; `api.` / `auth.foyr.online` → foyrd.
- Customer apps → `<app>.foyrapps.online` (`APPS_DOMAIN`), plus customer custom domains.

**Why:** If customer apps ran on `*.foyr.online`, any app (i.e. untrusted customer code) could set
cookies on `.foyr.online` (cookie tossing) and would count as *same-site* with the dashboard and auth
endpoints, weakening SameSite/CSRF protection. Vercel (`vercel.app`), GitHub (`github.io`) and
Netlify (`netlify.app`) separate customer content for the same reason. It also frees the apex for the
Vercel-hosted marketing site.

**Consequences:**
- Buy `foyrapps.online` before Phase 0 (not needed for the website). Name can change; everything reads `APPS_DOMAIN`.
- Add `foyrapps.online` to the Public Suffix List before public launch.
- Reserved slugs list enforced at app creation (see system design §7).

## ADR-016: Website stack: Next.js MDX docs, Resend waitlist
**Date:** 2026-09-26 · **Status:** Accepted · **Amends:** ADR-014, design brief §3 (Fumadocs)

**Decision:** `site/` is Next.js 16 + Tailwind v4 + Motion. Docs are plain MDX pages under
`app/docs/` rendered with Next's built-in MDX support and a custom shell (sidebar, table of
contents, ⌘K search, light/dark), not Fumadocs. Waitlist sign-ups go to a Resend audience through
a server action. Product mockups are generated from the Claude Design canvas by `scripts/dc2jsx.py`.

**Why:** The current Fumadocs release pulls in a new build toolchain (rolldown, satteri) for what is
ten pages; a custom shell matches the approved design exactly and has no framework to track.
Resend needs no database and is also the planned magic-link email provider.

**Consequences:** Docs search is a static index built from the MDX files at build time (fine for
tens of pages; revisit if docs grow past ~100). New docs pages must be added to `lib/docs-nav.ts`.

## ADR-017: Repository workflow: protected `main`, PR-only, CI on every PR
**Date:** 2026-09-26 · **Status:** Accepted

**Decision:**
- `main` is protected on GitHub. Every change goes through a pull request; direct pushes, force-pushes
  and deleting `main` are blocked, for admins too.
- Required approvals are **0** while there is one contributor (GitHub does not let you approve your own
  PR). Raise it to 1 when a second person joins. Stale approvals are dismissed on new pushes and all PR
  conversations must be resolved before merging.
- Every commit carries a `Signed-off-by` trailer (`git commit -s`), and commits are made as the personal
  account (see CLAUDE.md).
- CI is one workflow per part of the repo, path-filtered so unrelated PRs stay fast. Today:
  `.github/workflows/site-ci.yml` (lint, format, typecheck, build on Node 20 and 22). Go, OpenTofu and
  dependency-audit workflows are added with the backend.

**Why:** A hosting platform's `main` becomes what gets deployed; a mistake there should need a PR, a
passing check and a second look, even when the second look is the author's own diff. Protecting it now is
free and avoids retrofitting habits later.

**Consequences:**
- Required status checks are deliberately **not** enabled yet. Path-filtered workflows only run when their
  paths change, so a required check would block PRs that never trigger it. Enable them once either every
  check runs on every PR or each area has a stable always-running "gate" job.
- Branch protection lives in GitHub settings, not in git. To re-create it:

```bash
cat > protect.json <<'JSON'
{
  "required_status_checks": null,
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "required_approving_review_count": 0,
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false,
    "require_last_push_approval": false
  },
  "restrictions": null,
  "required_linear_history": false,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": true
}
JSON
gh api -X PUT repos/Arpit529Srivastava/Foyr/branches/main/protection --input protect.json
```

## ADR-018: Development lifecycle and testing standard
**Date:** 2026-09-27 · **Status:** Accepted

**Decision:** Every change goes plan, branch, tests, implementation, local checks, signed commit, PR, green CI, merge, post-deploy check. Code is not merged without tests: Go packages have `_test.go` files, bug fixes begin with a failing test, infra passes `tofu fmt -check` and `tofu validate`, and `tofu apply` only runs with explicit approval.

**Why:** foyrd and authproxy sit in front of other people's apps, so regressions are security problems. Tests written with the code are cheap; retrofitting them is not.

**Consequences:** CI per area (site, Go, later infra) must run these checks. Add coverage reporting and integration tests (Postgres, GitHub webhook fixtures) as those pieces land.

## ADR-019: Observability plan: CloudWatch now, OpenTelemetry-ready code, no self-hosted Prometheus/Grafana
**Date:** 2026-09-27 · **Status:** Accepted

**Decision:**
- Phase 0: CloudWatch Logs with retention on every log group, structured JSON logs (`log/slog`) with `request_id`, `app_id` and `deploy_id`, and CloudWatch alarms emailed through SNS (fck-nat down, builds failing repeatedly; later ALB 5xx).
- Phase 1: Sentry (free tier) in foyrd, authproxy and web; OpenTelemetry instrumentation in Go with a no-op exporter by default, switched on to X-Ray or Grafana Cloud when needed; an external uptime check on login and API.
- Phase 3: per-app request counts from authproxy (metering and dashboard), CloudWatch Live Tail for the logs viewer.
- No self-hosted Prometheus or Grafana. Revisit hosted Prometheus/Grafana when there is real traffic across many services.

**Why:** Self-hosting costs ~$15–30/month plus upkeep and gives nothing before there is traffic. Structured logs, request IDs and OpenTelemetry are cheap to add now and expensive to retrofit.

**Consequences:** Every new log group sets `retention_in_days`. Go services log JSON only and propagate a request ID.

## ADR-020: Build and run placement (step 4)
**Date:** 2026-09-27 · **Status:** Accepted

**Decision:**
- Step 4 is split. 4a (ECR, ECS cluster, CodeBuild, IAM, log groups, Cloud Map) is created now; 4b (ALB, platform RDS) is created when foyrd and authproxy exist, with a switch to turn them off when idle.
- CodeBuild runs **outside the VPC**, so untrusted build code never sits on the platform network or uses fck-nat.
- Foyr's own services run on **ARM64** (Graviton, ~20% cheaper). Customer apps build and run on **x86_64** by default for npm native-module compatibility; ARM can be offered later.
- Customer app tasks get no IAM task role. Platform and tenant tasks use separate execution roles, each limited to its own SSM path (`/foyr/platform/*`, `/foyr/apps/*`).
- Base images come from ECR Public (`public.ecr.aws/docker/library/...`) to avoid Docker Hub rate limits.

**Why:** Nothing needs the ALB or database until the Go services exist, saving ~$40/month. Keeping builds off the VPC is the simplest strong isolation for untrusted code.

**Consequences:** Phase 0's CodeBuild role can push to any `apps/*` repository; per-build scoping comes later. The ALB gets HTTPS once `foyrapps.online` is bought.

## ADR-021: Temporary site host and founder email until foyr.online is live
**Date:** 2026-09-29 · **Status:** Accepted (temporary) · **Amends:** ADR-015

**Decision:** Until `foyr.online` is bought (planned first week of October 2026):
- Every real link the site and its emails produce uses the Vercel URL `https://foyrai.vercel.app`. It is the default of `siteUrl` in `site/lib/site.ts` and the value of `NEXT_PUBLIC_SITE_URL` in Vercel. Docs text and product mockups that describe the future hosts (`app.foyr.online`, `*.foyrapps.online`) stay as they are, because they describe the product, not links.
- Waitlist notifications go to the founder address hardcoded in `site/app/actions/waitlist.ts` (`FOUNDER_EMAIL`), sent from Resend's shared `onboarding@resend.dev`. `WAITLIST_NOTIFY_EMAIL` still overrides it.
- The welcome email to people who join stays off until a sending domain is verified.

**Why:** The domain isn't owned yet, so `foyr.online` links would be dead, and Resend can only deliver to the account owner without a verified domain.

**Consequences:** These are deliberate exceptions to "never hardcode domains". Every one is listed in `docs/domain-switch.md`, which must be worked through when the domain is bought, and this ADR is then superseded.

## ADR-022: foyr.online is live; site URL default switched back
**Date:** 2026-10-10 · **Status:** Accepted · **Supersedes:** ADR-021 (site URL part)

**Decision:** `foyr.online` was bought on 2026-10-10 (GoDaddy). The `siteUrl` default in `site/lib/site.ts` is `https://foyr.online` again and `NEXT_PUBLIC_SITE_URL` is set to it in Vercel; `foyrai.vercel.app` redirects to it. Still temporary under ADR-021 until their own steps are done: the hardcoded `FOUNDER_EMAIL` (until a `@foyr.online` mailbox exists) and the welcome email (until `mail.foyr.online` is verified in Resend). `docs/domain-switch.md` tracks what is left.
