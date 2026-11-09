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

## ADR-023: GitHub App setup for Phase 0 (step 5)
**Date:** 2026-10-10 · **Status:** Accepted · **Amends:** ADR-010, roadmap Phase 0

**Decision:**
- Two GitHub Apps: **`foyr-dev`** now (webhooks through a smee.io relay to foyrd on a laptop, installed only on test repos) and **`Foyr`** for production once `api.foyr.online` serves foyrd. Customers never see the dev App, and a leaked dev key cannot read customer repos.
- Permissions: Contents read-only and Metadata read-only. Events: push (installation events arrive automatically).
- Step 5 only receives and verifies webhooks and mints installation tokens. Saving installations and pushes moves to step 6 (with the platform database), and **developer login with GitHub moves to Phase 1**, where the dashboard and sessions live.
- The App JWT is signed with the standard library (RS256); no JWT library until Phase 1 needs to verify untrusted tokens.

**Why:** Keeps step 5 small and testable, keeps test traffic away from the production App, and adds no dependencies.

**Consequences:** smee.io sees webhook payloads (repo names, commit metadata, never code or secrets); signatures are still verified, so it cannot forge events. Acceptable for test repos only. The App private key lives outside the repo locally and in SSM `/foyr/platform/` in AWS.

## ADR-024: Deployments are fully visible to the developer
**Date:** 2026-10-10 · **Status:** Accepted

**Decision:** Every deployment records enough for a developer to debug it without asking us, shown later in the dashboard like a CI run (GitHub Actions style): status, who pushed, commit and message, each step (clone, detect, build, push image, and later release) with start, end and duration, a timeline of status changes, the failed step and the reason, and a link to the full build log in CloudWatch. The data is recorded from the first build (step 6); the dashboard pages come in Phase 1.

**Why:** Developers trust a platform they can see into. "It failed" with no reason means a support ticket; "step `docker build` failed: missing script build" means they fix it themselves.

**Consequences:** Tables `deployments`, `deployment_steps`, `deployment_events`. Build logs stay in CloudWatch (large, streamed); Postgres keeps only pointers and short reasons.

## ADR-025: Step 6 building blocks
**Date:** 2026-10-10 · **Status:** Accepted · **Amends:** system design §5, §10

**Decision:**
- Platform data in **Postgres 17** via **pgx v5** (no ORM). Local Homebrew Postgres in development, RDS when step 4b lands.
- Migrations are plain SQL files embedded in the foyrd binary (`internal/db/migrations`), applied at startup under an advisory lock and recorded in `schema_migrations`. They live under `internal/db` (not top-level `db/`) because Go can only embed files inside the package directory.
- The job queue is a Postgres table using `FOR UPDATE SKIP LOCKED` with leases, retries with backoff and a max of 3 attempts. Platform errors (AWS throttling) are retried; customer build failures are not.
- Push webhooks for the repo's default branch create a deployment; other branches and tags are recorded as ignored. A callback error answers 500 so GitHub can redeliver; the commit's unique index makes redelivery safe.
- Step 6a: database, store, queue, webhook → deployment. Step 6b: worker, CodeBuild, ECR, SSM adapters and the build itself.
- Tests run against a real Postgres (each test in its own schema). CI has a Postgres service; locally, tests skip without `FOYR_TEST_DATABASE_URL`.

**Why:** One database we already need, no extra queue service, simple SQL we can read, and tests that exercise real locking.

**Consequences:** Go CI needs a Postgres service. Revisit the queue (move to SQS behind the same worker interface) only at thousands of jobs per minute.

## ADR-026: Audit trail, traceability and scale
**Date:** 2026-10-10 · **Status:** Accepted

**Decision:**
- Three kinds of records: the **deployment trail** (ADR-024), an **audit log** of who did what (`audit_events`), and later **access logs** of requests through authproxy (CloudWatch then S3, never Postgres).
- `audit_events` is **append-only** (a trigger rejects UPDATE, DELETE and TRUNCATE) and **partitioned by month**. foyrd creates the next months' partitions at startup and daily; a default partition catches anything else.
- Every state change writes its audit event **in the same transaction**, so the trail can never miss an action. Actor is a GitHub user, a Foyr user (later) or the system.
- A **trace ID** (the webhook request ID) is stored on the deployment, its job, its audit events and the delivery row, and is in every log line.
- Every verified webhook is stored in `webhook_deliveries` (result, detail, duration, attempts) for 30 days. Unverified requests are never stored.
- Every tenant-owned row carries the **tenant key** (`installation_id`, later `team_id`), and indexes start with it. Lists use cursor pagination.
- **Fairness:** at most 2 running jobs per installation (`DefaultMaxRunningPerTenant`), enforced per tenant with an advisory lock so concurrent workers cannot exceed it.
- **Retention:** deployments and audit events kept; finished jobs pruned after 7 days; deliveries after 30 days. Later, per-plan audit retention (dropping old partitions), customer-chosen access-log detail (off, basic, with user identity) because it holds their end users' personal data (DPDP Act, GDPR), and export (CSV/JSON, then streaming to the customer's own tools on paid plans).

**Why:** Teams with many users need to answer "who did what, when, and why did it break" themselves. Retrofitting audit, tracing, tenant keys and partitions later means downtime; adding them now costs little.

**Consequences:** Postgres holds small, important facts; anything large or high-volume goes to services built for volume. Scaling path in system design §12.

## ADR-027: The build worker (step 6b)
**Date:** 2026-10-10 · **Status:** Accepted · **Builds on:** ADR-020, ADR-024, ADR-025

**Decision:**
- foyrd runs a **worker pool** (`FOYR_WORKERS`, default 2; 0 turns building off). Each worker claims a job, renews its 2-minute lease every 30 s (heartbeat), and completes, retries or, on shutdown, hands the job back without counting the attempt. If the lease is lost, the job is cancelled because another worker owns it.
- One build = one run of the shared CodeBuild project `foyr-build`. foyrd passes `REPO_URL`, `COMMIT`, `IMAGE_URI` and the **name** of an SSM parameter holding the clone token (`/foyr/builds/d-<id>/token`, SecureString). CodeBuild reads it as a `PARAMETER_STORE` variable, so the value is masked in logs. foyrd deletes it once the clone step is over, or when the build ends. If foyrd dies first, the token still expires within an hour.
- The clone sends the token in an `http.extraHeader`, never in the URL, so it is not written to `.git/config` or printed in errors.
- **Steps come from CodeBuild phases**, not from markers in the log: Prepare (submitted to install), Clone (PRE_BUILD), Build image (BUILD), Push image (POST_BUILD). They are saved on every poll (10 s) with times and the log link. Build and clone abort on failure, so later steps show as skipped.
- **Resume, don't restart:** the CodeBuild ID is saved as soon as the build starts. A job picked up again after a crash or restart watches that build instead of starting a second one.
- **Who is at fault decides retries.** Platform errors (AWS throttling, network) are retried after 30 s, then 2 min, at most 3 attempts. After that the deployment fails with "this is on our side". Customer failures (build error, timeout, no Dockerfile, App removed from the repo) are recorded at once with the failed step, the reason and the last 30 log lines, and are not retried.
- **Images:** one ECR repository per app, `apps/<app id>`, created by foyrd on first build with immutable tags, scan on push and a keep-last-10 lifecycle rule. Tag `d<deployment id>-<12-char commit>` is unique per deployment, so a tag is never overwritten.
- **What gets built:** the repo's `Dockerfile` if present (any language). Otherwise, if `package.json` uses `next`, Foyr's Next.js template (`templates/nextjs/Dockerfile`, embedded into the buildspec by OpenTofu). Otherwise the build fails with a message asking for a Dockerfile.
- foyrd checks its AWS credentials at startup (STS) and refuses to start workers without them. Locally it uses the `foyr` SSO profile. In AWS (step 4b) it gets a task role with exactly these permissions: CodeBuild start and read, ECR create and describe repo plus lifecycle, SSM put and delete under `/foyr/builds/`, and read build logs.

**Why:** The shortest path to a reliable, debuggable build: no extra services, the token never touches disk or logs, crashes don't lose or duplicate builds, and customers see exactly which step failed and why.

**Consequences:** CodeBuild's role gains `ssm:GetParameters` on `/foyr/builds/*` only. The CodeBuild concurrent-build quota caps parallel builds account-wide; raise it when needed.

## ADR-028: Customer apps on `apps.foyr.online` until the first outside user
**Date:** 2026-10-10 · **Status:** Accepted (temporary) · **Amends:** ADR-015

**Decision:** Until the separate apps domain is bought, `APPS_DOMAIN` = `apps.foyr.online`, so apps are served at `<app>.apps.foyr.online`. Only the founder's own apps run there. **Before the first outside person deploys an app**, buy a separate registrable domain (`foyrapps.online`, or a cheaper extension such as `.com`), switch `APPS_DOMAIN`, and add it to the Public Suffix List. Planned for Oct 24 to 30 (docs/yc-plan.md). Locally, apps use `<app>.localhost`.

**Why:** The domain costs cash the founder doesn't have this month, while AWS credits cover everything else. ADR-015's risks (cookie tossing, same-site CSRF, a phishing app getting `foyr.online` blocked) all come from untrusted code; with only the founder's code running, they don't apply. `mikeai.in` was considered and rejected: it is a live product with its own users, and sharing a site would put its sign-ins and reputation at risk.

**Consequences:** `APPS_DOMAIN` stays a setting, never hardcoded, so switching is a config change plus a DNS zone and a free wildcard certificate. App URLs change at the switch, which is harmless before outside users. Tracked in docs/domain-switch.md.

## ADR-029: Running deployments (step 7)
**Date:** 2026-10-10 · **Status:** Accepted · **Builds on:** ADR-020, ADR-027

**Decision:**
- A finished build queues a `deploy` job in the same transaction. The deploy job registers a task definition (family `app-<app id>`, one essential container `app`, x86_64, 0.25 vCPU / 512 MB, the tenant execution role, **no task role**) and creates one ECS service per deployment, `d-<deployment id>`. The service runs one task on **Fargate Spot** (`FOYR_APPS_CAPACITY`), in the tenant subnets with no public IP, and registers as `d-<id>.apps.foyr.internal` in Cloud Map for authproxy (step 8). Both calls are idempotent by name, and the service name is saved so a restarted job resumes instead of starting a second service.
- **Health check:** ECS's container health check runs inside the task, so no load balancer is needed. It passes when `/api/health` or `/` answers 2xx on `PORT` (3000), on `127.0.0.1` or on the task's own address (`$HOSTNAME`), using whichever of wget, curl or node the image has (all Node and Alpine images). The second address matters because Fargate sets `HOSTNAME` to the task's name, overriding the image's `HOSTNAME=0.0.0.0`, so Next.js listens only on the task address (found in the first live deploy, deployment 3). Images without any of them need a later network check from authproxy (step 8).
- Statuses: `built → deploying → live`, or `failed`. The previous live deployment becomes `superseded` and is retired (service and Cloud Map name deleted) by a `retire` job **5 minutes** later, so in-flight requests finish. If an older deployment becomes healthy after a newer one is live, it is superseded at once: the newest commit always wins. `apps.live_deployment_id` points at the live one.
- Failures are recorded, not retried: a task that stops (crash, out of memory, image pull error) fails the deploy at **Start** or **Health check** with the reason, the exit code and the last 30 lines of the app's log. A task that isn't healthy within 6 minutes fails with "timed out". The failed service is deleted and the previous live version keeps serving; a `retire` job one minute later removes anything AWS would not delete yet (e.g. the Cloud Map name while the task deregisters). Platform errors (ECS throttling) are retried like builds.
- App output goes to one log group, `/foyr/apps` (14 days), one stream per task (`app-<id>/app/<task>`). Each step links to its stream.
- New tenant security group `foyr-tenant`: outbound only, no inbound until authproxy exists.

**Why:** The shortest safe path to a running app: no load balancer per app (quotas, cost), crashes explained in the developer's terms, and blue/green that never takes a working version down.

**Consequences:** One small Spot task per live app (a few dollars a month each, from credits). foyrd needs the runtime settings from `tofu output`; `scripts/infra-env.sh` writes them. In AWS (step 4b) foyrd's task role adds ECS register, create, describe and delete, Cloud Map create, get, list and delete, and `iam:PassRole` on the tenant execution role only.

## ADR-030: authproxy routing (step 8)
**Date:** 2026-10-10 · **Status:** Accepted · **Builds on:** ADR-006, ADR-028, ADR-029

**Decision:**
- Every app gets a **slug** from its repo name (`octo/Foyr_Hello` → `foyr-hello`): 3 to 40 of `a-z0-9-`, never a reserved word (`app`, `api`, `auth`, `www`, `docs`, `admin`, `status`, `mail`, `static`, `cdn`, `foyr`), unique (`-2` … `-9`, then `app-<id>`), and kept when the repo is reconnected. The app is served at `<slug>.<APPS_DOMAIN>` (`foyr-hello.apps.foyr.online`, ADR-028).
- foyrd serves `GET /internal/routes` on a **separate private listener** (`FOYR_INTERNAL_ADDR`, `:9090`) that the public load balancer never forwards to, protected by a shared bearer token (`FOYR_INTERNAL_TOKEN`, 32+ characters, constant-time compare). It lists every connected app with a live deployment: slug → `d-<id>`.
- authproxy polls it every 3 s into an in-memory table swapped atomically. On any error it **keeps the last good table**, so apps keep serving while foyrd is down or redeploying. Until the first load, its health check (`/_foyr/healthz`, answered on every host) returns 503, so the load balancer doesn't send traffic to an empty proxy.
- Each request: hostname → slug → `d-<id>.apps.foyr.internal:3000`. The proxy **removes every `X-Foyr-*` header** visitors send (Phase 1 adds the trusted ones), keeps the load balancer's `X-Forwarded-For` and `X-Forwarded-Proto`, preserves `Host`, passes `X-Request-Id`, streams responses, and uses dial and response-header timeouts. Unknown hosts get a branded 404, unreachable apps a branded 502.
- Locally, `scripts/authproxy-dev.sh` serves `<slug>.apps.localhost:8081`; `FOYR_STATIC_ROUTES` points a slug at a local app.

**Why:** No per-app load balancer rules or target groups (quotas), instant blue/green by changing one pointer, and the front door keeps working when the control plane doesn't.

**Consequences:** In AWS, authproxy's security group may reach foyrd on 9090 and tenant tasks on 3000; nothing else may. A new route takes up to 3 s to appear.

## ADR-031: The public front door (step 8b)
**Date:** 2026-10-18 · **Status:** Accepted · **Builds on:** ADR-008, ADR-015, ADR-028, ADR-030

**Decision:**
- **DNS:** `apps.foyr.online` is a Route 53 hosted zone, delegated from GoDaddy with 4 NS records. `foyr.online` itself stays at GoDaddy. One record, `*.apps.foyr.online`, points every app at the load balancer.
- **TLS:** one ACM wildcard certificate, `*.apps.foyr.online`, validated by DNS in that zone and renewed by AWS.
- **Load balancer:** one public ALB in the public subnets. 443 uses `ELBSecurityPolicy-TLS13-1-2-2021-06` and forwards everything to authproxy; 80 only answers 301 to HTTPS. It drops malformed headers. Health check: `/_foyr/healthz`.
- **authproxy** runs as ECS service `authproxy` on Fargate ARM64 (256/512, on-demand, not Spot: it is the front door) in the platform subnets, no public IP, task family `foyr-authproxy`, logs in `/foyr/platform` (prefix `authproxy`). Deployment circuit breaker with rollback.
- **Security groups:** internet → ALB 80/443; ALB → authproxy 8081 only; authproxy → tenant 3000 and HTTPS out (image pulls, logs). Tenant tasks accept only authproxy on 3000. The tenant NACL already lets replies back.
- **Images:** GitHub Actions (`.github/workflows/authproxy-deploy.yml`, now `platform-deploy.yml`, ADR-032) on pushes to `main` tests, cross-compiles `linux/arm64`, packages the binary on distroless `static-debian12:nonroot`, pushes `foyr/authproxy:<sha12>`, registers a new revision of the latest task definition with only the image changed, and waits for the service to be stable. It signs in with **OIDC** through the account's existing GitHub provider (shared with another project, so tofu only reads it): role `foyr-github-deploy` trusts only `repo:Arpit529Srivastava@151747267/Foyr@1388194589:ref:refs/heads/main` (GitHub's immutable subject with owner and repo IDs, which this repo uses), can push only to `foyr/authproxy`, update only the `authproxy` service, and pass only the platform execution role. No AWS keys are stored in GitHub.
- **Routes until 8c:** foyrd still runs on the laptop, so authproxy in AWS uses `FOYR_STATIC_ROUTES` (`authproxy_static_routes`, today `foyr-hello=d-4.apps.foyr.internal:3000`). A new deploy of foyr-hello needs this value changed until 8c switches authproxy to foyrd's `/internal/routes`.

**Why:** The cheapest setup that gives every app a real HTTPS address today: one ALB (about $17/month, from credits) for all apps, a free certificate, $0.50/month DNS. Delegating only the `apps` subdomain lets AWS manage the records the load balancer and certificate need without moving the website's DNS.

**Consequences:** tofu owns the settings of the first task definition; the pipeline owns which image runs (`ignore_changes = [task_definition]`). After changing authproxy settings in tofu, run the pipeline (`workflow_dispatch`) so a revision with the new settings and the current image is deployed. Moving to the separate apps domain (ADR-028) means a new zone and certificate, listed in docs/domain-switch.md.

## ADR-032: foyrd in AWS (step 8c)
**Date:** 2026-10-18 · **Status:** Accepted · **Builds on:** ADR-020, ADR-023, ADR-026, ADR-030, ADR-031

**Decision:**
- **Database:** RDS Postgres 17, `db.t4g.micro`, 20 GB gp3 (grows to 50), single-AZ, encrypted, private (platform subnets), 7 days of backups, deletion protection, final snapshot. Only foyrd's security group reaches 5432. The password is generated by tofu (40 URL-safe characters) and stored with the connection string in SSM `/foyr/platform/database-url` (SecureString). It is also in the encrypted, private state bucket; acceptable for now, revisit with write-only arguments.
- **Secrets:** the GitHub webhook secret, the App private key and the internal token go into SSM `/foyr/platform/*` with `scripts/platform-secrets.sh`, straight from `~/.config/foyr`, so they are never in tofu state, the repo or the terminal. ECS injects them through the platform execution role; app code never sees AWS credentials for them.
- **foyrd** runs as ECS service `foyrd` on Fargate ARM64 (256/512, on-demand) in the platform subnets, no public IP, 2 build workers, the same foyr-dev GitHub App (ADR-023). Its task role `foyr-foyrd` may only: start and read builds of `foyr-build`; create `apps/*` repositories; write and delete `/foyr/builds/*` tokens; read build and app logs; register task definitions; create, describe and delete `d-*` services (never `foyrd` or `authproxy`); manage Cloud Map names; pass only the tenant execution role.
- **Public:** `api.foyr.online` (Route 53 zone delegated from GoDaddy, own ACM certificate added to the same HTTPS listener by SNI). A listener rule sends that host to foyrd's port 8080; every other host stays with authproxy. The internal listener (9090) is never a load balancer target.
- **Routes:** foyrd registers as `foyrd.platform.foyr.internal` (Cloud Map). authproxy polls `http://foyrd.platform.foyr.internal:9090/internal/routes` with the token from SSM; only authproxy's security group reaches 9090. The static-route bridge from ADR-031 is removed.
- **Deploys:** `.github/workflows/platform-deploy.yml` deploys foyrd, then authproxy (a new authproxy only turns healthy once foyrd answers), using the same OIDC role, now allowed `foyr/foyrd`, service `foyrd` and passing `foyr-foyrd`.
- **Data:** the local `foyr_dev` database is copied once into RDS with `scripts/db-import.sh` before foyrd first starts there, so app and deployment IDs continue (no clash with existing `apps/1` image tags or `d-4`). A one-off Fargate task inside the VPC restores it in a single transaction; the dump is staged in S3 for 15 minutes.

**Why:** `git push` → live URL must work with the laptop off, and new deployments must reach the URL by themselves. One small database, one small task and the existing load balancer keep the cost at about $20/month from credits.

**Consequences:** Once the App's webhook URL points at `api.foyr.online`, local foyrd receives no webhooks, but its workers could still pick up old jobs from the local database and create `d-<id>` services that clash with the ones foyrd in AWS owns. Run local foyrd with `FOYR_WORKERS=0`, or with a separate dev App. Changing foyrd or authproxy settings in tofu takes effect at the next pipeline run (run it by hand from the Actions tab).

## ADR-033: Founder-only page
**Date:** 2026-10-19 · **Status:** Accepted · **Builds on:** ADR-026, ADR-032 · **Replaced by:** the customer dashboard (Phase 1)

**Decision:**
- foyrd serves a read-only page at `https://api.foyr.online/founder`: apps with their live address and deployment, the last 30 deployments (status, commit, push-to-live time, failure reason), and per deployment its build steps with timings, timeline, failure reason with the app's last log lines, and audit trail. Server-rendered Go templates, no JavaScript, in the website's colors (black, grey cards, amber `#f5a524`, the door logo). A left sidebar (Overview, Deployments, Activity, then each app with a live dot; a top bar on phones) frames every page. Deployments lists all deployments or one app's (`?app=<id>`); Activity is the audit log. The overview leads with median push-to-live time and a bar chart of recent deployments, then the latest pipeline, a 7-day activity grid (IST) and recent deployments; the deployment page shows where the time went as a step timeline. One entrance animation (bars grow, grid fills, steps sweep), skipped while the page refreshes and for reduced motion. It refreshes every 5 s while a deployment younger than 30 minutes is still running.
- **Access:** one shared key, `FOYR_FOUNDER_KEY` (64 hex characters, SSM `/foyr/platform/founder-key`, made by `scripts/platform-secrets.sh` into `~/.config/foyr/founder-key`). The page is off (404) without a key of 32+ characters. The key is pasted into a sign-in form (POST), never put in a URL, so it never reaches browser history or request logs. `scripts/founder-key.sh` copies it to the clipboard and opens the page.
- **Session:** cookie `__Host-foyr_founder` (Secure, HttpOnly, SameSite=Strict, host-only) holding `v2.<expiry>.<random id>.<HMAC-SHA256>` signed with the key. The server checks the MAC in constant time and the expiry (24 hours); a changed, expired or old-format cookie means signing in again, and a new key signs everyone out.
- **Brute force:** each attempt is counted atomically when it starts (and forgiven if the key was right), so parallel requests can't slip past the check. 5 attempts from one address in 15 minutes lock that address out (429 with `Retry-After`). Past 30 attempts from anywhere, sign-in enters a slow mode of one attempt per 10 seconds for everyone, not a lockout, so strangers can't lock the founder out. Every wrong key waits 500 ms. The address is the rightmost `X-Forwarded-For` entry, the one the load balancer appends, so visitors can't spoof it. Counters live in memory (one foyrd task), are capped at 10,000 addresses, and reset when foyrd restarts or deploys; acceptable against a 256-bit key, move them to Postgres if the page gains accounts.
- **Forms:** sign-in and sign-out accept only same-origin posts (the `Origin` header must match the host); bodies are capped at 4 KB.
- **Headers:** `Content-Security-Policy: default-src 'none'; script-src 'nonce-<random per page>'` (the only script is the sidebar toggle; inline styles allowed), `Strict-Transport-Security: max-age=31536000; includeSubDomains`, `X-Frame-Options: DENY`, `Cross-Origin-Opener-Policy: same-origin`, `Referrer-Policy: same-origin` (`no-referrer` made browsers send `Origin: null` on the page's own forms), `no-store`, `noindex`, `nosniff`.
- **Sidebar:** slides away with a toggle or the `[` key and stays hidden through refreshes via a non-secret `foyr_sidebar` cookie.

**Why:** Watching deploys in a browser instead of CloudWatch, and real screenshots for the YC application, without building accounts before magic-link login exists.

**Consequences:** Anyone with the key sees every app's deployments; it is for the founder only and goes away when the Phase 1 dashboard has real logins. Rotating the key (signs everyone out): delete `~/.config/foyr/founder-key`, run `scripts/platform-secrets.sh`, then the Deploy platform workflow. Sessions can't be revoked one by one; rotation is the kill switch. A second factor (TOTP) is the next step if the page ever shows more than deploy metadata.

## ADR-034: Customer dashboard served by foyrd
**Date:** 2026-10-19 · **Status:** Accepted · **Builds on:** ADR-033 · **Replaces:** the Next.js `web` service in system-design.md, for now

**Decision:**
- The customer dashboard is server-rendered Go templates in foyrd (`internal/dashboard`), at `/dashboard/<account>/<app>/<tab>`, not a separate Next.js service. It reuses the founder page's security headers and design tokens (`internal/ui`).
- Pages: an account page (apps as cards, recent deployments) with an account switcher and an app search box (⌘K), and per app the tabs Overview, Deployments, Logs, Metrics, Access, Environment, Domains and Settings. Overview, Deployments (with a status filter and a deployment page that explains failures in plain words), Domains and Settings show real data. Logs, Metrics, Access and Environment show clearly labeled sample data until their features exist (spec: docs/superpowers/specs/2026-10-19-deploy-anything-design.md).
- Every lookup is scoped: an app is found only inside its account (`installation_id`), and a deployment only inside its app, so another account's ids answer 404.
- Until magic-link login exists, the dashboard borrows the founder sign-in (`Allowed` = founder session) and the founder can open every account. Login replaces `Allowed` with the customer's session and limits the switcher to their own accounts.

**Why:** One language, one deploy, and the security work already done; it ships the UI in days, which matters before the YC deadline. It can move to a client-side app later if the dashboard needs heavy client state.

**Consequences:** The `web` ECR repository stays unused for now. Dashboard changes deploy with foyrd.

**Update 2026-11-09:** the Access tab is real now (access mode, people, invites, revoke, remove), behind the founder sign-in, with every change scoped to the app's account and written to the audit trail as `founder`. API tokens and login-skipping paths there are marked Coming soon. Customer sign-in to the dashboard is still to do.

## ADR-035: Magic-link login
**Date:** 2026-10-25 · **Status:** Accepted · **Builds on:** ADR-006, ADR-026, ADR-034 · **Spec:** docs/superpowers/plans/2026-10-19-login.md

**Decision:**
- **Flow:** a visitor of a private app is sent by authproxy to `auth.<PLATFORM_DOMAIN>/login?app=<slug>`, enters an email, gets a link, presses **Continue**, and lands on `<slug>.<APPS_DOMAIN>/__foyr/callback?code=…`. authproxy swaps the one-time code at foyrd (private listener, internal token) for a signed session and sets `__Host-foyr_session` (host-only, HttpOnly, Secure, SameSite=Lax). Every app host has its own cookie, which is what makes custom domains work later (system-design 4c).
- **Resend, not SES, for now.** No SES sandbox wait before the deadline. Mail goes from `login@mail.<PLATFORM_DOMAIN>`, the subdomain already verified in Resend. Locally the log sender prints the email instead (only with a `http://localhost` auth URL).
- **Our own session token** (`v1.<payload>.<Ed25519 signature>`, ~60 lines in `internal/authn`) instead of a JWT library: one algorithm, no `alg` header to confuse, signature over a fixed context string. foyrd holds the seed (`FOYR_SESSION_KEY`); authproxy holds only the public key. Sessions last 7 days and carry `iat` and `exp`.
- **The link opens a button, not a login.** Mail scanners (Outlook, Gmail) prefetch links, so a GET of `/login/verify` never touches the token; only the POST does. Tokens (15 minutes) and codes (60 seconds) are 32 random bytes, stored only as SHA-256, single use, spent with one `UPDATE … WHERE used_at IS NULL`. The verify page widens CSP `form-action` to the apps domain because Chrome applies it to the redirect after a post.
- **The form never says who is a member.** A member, a stranger and an unknown app get byte-identical answers; the email is sent in the background so response time doesn't tell either. 3 emails per address and 10 posts per IP per 15 minutes.
- **Access:** per app `private` (default) or `public`; roles `owner > admin > member > viewer`. Apps that existed when migration 005 ran became `public`, so foyr-hello keeps serving until the founder switches it. authproxy polls `/internal/access` every 3 s (like routes) and keeps the last good snapshot if foyrd is down; removals and revoked sessions apply within one poll. An app the snapshot doesn't know yet is treated as private. A session younger than 20 s whose user isn't in the snapshot yet gets a self-refreshing 503 instead of a 403 (found by running the flow locally: the first page after login was a 403).
- **What the app gets:** `X-Foyr-User-Id`, `X-Foyr-Email`, `X-Foyr-Role`; visitor-sent `X-Foyr-*` headers and the session cookie never reach it. Signed-in non-members of a public app get role `viewer`.
- **Hosts:** `auth.` and `app.` share one ACM certificate and go to foyrd through the existing load balancer. `foyr.online` stays at GoDaddy, so the validation records and the two CNAMEs are added by hand; `platform_hosts_live` keeps the attach step out of the first apply so it doesn't block waiting for DNS.
- **Keys:** `go run ./cmd/sessionkey` makes the pair (macOS's openssl cannot make Ed25519 keys); `scripts/platform-secrets.sh` keeps it in `~/.config/foyr/session.{key,pub}` and SSM `/foyr/platform/session-key`, `session-public-key`, `resend-api-key`.
- Every state change writes its audit event in the same transaction: `invite.created`, `invite.revoked`, `member.removed`, `app.access_changed`, `user.signed_in`.

**Why:** Login in front of every app is the product's wedge and the second part of the YC demo (push, live URL, login, invite).

**Consequences:** Rotating the session key signs everyone out. A new foyrd needs the three SSM parameters before it deploys or its task won't start (the circuit breaker rolls back). Roles are stored and sent but not enforced beyond membership until Phase 2 (`hasPermission`). foyrd must be reachable from authproxy for new logins and removals; existing sessions keep working without it.

## ADR-036: Customer sign-in to the dashboard
**Date:** 2026-11-09 · **Status:** Accepted · **Builds on:** ADR-034, ADR-035 · **Plan:** docs/superpowers/plans/2026-11-09-dashboard-login.md

**Decision:**
- The dashboard gets its own magic-link login and its own tables (`dashboard_login_tokens`, `dashboard_sessions`), separate from app login. An app visitor's session can never open the dashboard, and the other way round.
- A person may open an account's dashboard only through a link row in `account_members` (`installation_id`, `user_id`, role owner, admin or member). Only the founder creates links for now; GitHub sign-in (ADR-023) can create them later. Being invited to an app or being its member gives no dashboard access.
- The session is an opaque random value in a host-only cookie. Only its SHA-256 is stored, and every request reads the row, so logout and removing the last link end a session at once. 7 days.
- Login tokens work like app login (15 minutes, single use, Continue button against mail scanners, the form never says who has an account). An email is sent only when it is linked to at least one account.
- Roles: owner and admin may change things, member is read-only, checked on every post. The audit trail names the person's email.
- Order of work: tables and store, then scoping every dashboard page to the person's accounts (still founder-only), then the sign-in pages. The boundary lands before anyone can sign in.

**Why:** A developer you onboard must see only their own account. One shared login path for apps and the dashboard would let an app visitor become a dashboard user.

**Consequences:** Migration 006 is additive (three new tables) and foyrd applies it on start. Nothing changes for users until the sign-in pages ship.

**Update 2026-11-09 (step 2):** the dashboard takes a `Person` (founder, or a customer with a role per account) instead of a yes or no. Accounts the person is not linked to do not exist for that request, so every page and post answers 404 for them. A member's posts answer 403 after that, and the audit trail names the person. Wired to the founder sign-in only, so users see no change yet.

**Update 2026-11-09 (step 3):** customers can sign in. Pages: `/dashboard/login`, `/dashboard/login/verify`, `/dashboard/logout`, served only on the dashboard host (`https://app.<PLATFORM_DOMAIN>`, taken from `FOYR_AUTH_URL` like the site URL; `http://localhost:<port>` in development), where `/` goes to `/dashboard`. Cookie `__Host-foyr_dash`. The founder links an email to an account at `/founder/accounts`. A signed-out visitor on the dashboard host is sent to the customer sign-in; on any other host to the founder sign-in. Rate limits as for app login (3 emails per address, 10 posts per IP, 15 minutes), now in `web.Window`. A GitHub account named `login` would be shadowed by `/dashboard/login`.

## ADR-037: One console design, self-hosted fonts

**Decision:**
- The customer dashboard, founder page and sign-in pages share one stylesheet, `internal/ui/console.css`, put in each page by `ui.ConsoleHead`. A page's own `<style>` holds only what is unique to it.
- The look follows foyr.online (black, amber `#f5a524`, Inter, Geist Mono), with greys warmed toward the amber. Panels share hairlines, corners stay at 8px or less, status marks are squares taken from the logo's pixels. Gradients appear only on the primary action and on things that are live or measured. Mono is for machine values such as commits and addresses.
- An app's deployments are drawn as a ledger of squares, and the one serving traffic is lit.
- foyrd serves Inter and Geist Mono (latin, variable, SIL OFL, licences beside the files) at `/assets/`, cached for 30 days. The CSP gains `font-src 'self'`; no third-party font host is allowed.

**Why:** Two copies of the CSS had drifted, and the pages named Inter without ever loading it, so most visitors got a system font. Self-hosting keeps `default-src 'none'` meaningful and sends no visitor data to a font CDN.

**Consequences:** About 80 KB of fonts per browser, once a month. foyr.online is unchanged and keeps its own Next.js fonts. New console pages reuse `.card`, `.sheet`, `.list`, `.steps` before adding CSS.

**Update 2026-11-09 (themes):** the console has a light theme. Each colour token holds both values with CSS `light-dark()`, and `data-theme="light"` on `<html>` switches `color-scheme`. Dark stays the default, as on foyr.online. The sidebar button saves the choice in `localStorage` (per host), and a script allowed by the page's nonce applies it before first paint. Magic-link sign-in pages (app login and dashboard sign-in) have no switch and stay dark, because their answers must be byte-identical whether or not an email has an account, and a nonce in the body would break that. Content fills the window.

## ADR-038: Identity contract for apps
**Date:** 2026-11-09 · **Status:** Accepted · **Builds on:** ADR-006, ADR-035 · **Spec:** docs/superpowers/specs/2026-11-09-auth-contract-design.md

**Decision:**
- **The assertion is an ES256 JWS.** `X-Foyr-Assertion` is a compact JWT signed with ECDSA P-256 and SHA-256 (signature is the 64-byte `r||s` of RFC 7515). Claims: `iss` (the auth URL), `aud` (the app slug), `sub` (the user's public id), `email`, `role`, `iat`, `exp`. It lasts 60 seconds. Header members are `alg`, `typ` and `kid`.
- **Opaque public user ids.** Migration 007 adds `users.public_id`, `usr_` plus 16 lowercase hex characters, unique and checked by the database. The internal numeric id never leaves foyrd. `X-Foyr-User-Id` and `sub` carry the public id.
- **authproxy has its own signing key.** `FOYR_ASSERTION_KEY` (base64 PKCS#8 P-256) goes to authproxy only, so authproxy still cannot create sessions (ADR-035). foyrd holds only public keys in `FOYR_ASSERTION_PUBLIC_KEYS` (comma-separated base64 SPKI) and publishes them at `GET /.well-known/jwks.json` on the auth host. Each key's `kid` is its RFC 7638 thumbprint, so the id follows the key and needs no registry. The response is `Cache-Control: public, max-age=300`, `Access-Control-Allow-Origin: *`, keys sorted by `kid`.
- **`pid` in the session.** The session token now carries the user's public id, so authproxy can sign without asking foyrd. A session without `pid` (issued before this change) is treated as signed out: the visitor is sent to sign in once, and the new session has it.
- **Environment for apps.** `internal/run` gives every deployment `FOYR_APP_SLUG` and, when login is on, `FOYR_AUTH_URL`. The `FOYR_` prefix is reserved for the platform: the future Environment tab must refuse customer variables that start with it.
- **Four headers.** `X-Foyr-User-Id`, `X-Foyr-Email`, `X-Foyr-Role`, `X-Foyr-Assertion`. Visitor-sent `X-Foyr-*` headers are still deleted. The first three are for reading; an app that must not trust the network verifies the fourth.
- **Shared vectors.** `sdk/vectors/assertion.json` and its README are the conformance suite every SDK must pass. The Go verifier in `internal/authn` is the reference.

**Implementation rulings (part of the contract, they refine the spec):**
- **A. Verifier strictness.** Known member names match exactly (case-sensitive). Duplicate member names are rejected in the header and the payload. Trailing data after the JSON object is rejected. Unknown header members are rejected. Unknown payload claims are ignored, so a claim added later cannot break deployed SDKs. Each segment may contain only `[A-Za-z0-9_-]`.
- **B. High-S signatures are accepted** (RFC 7515 allows both forms). Apps must never use the token string as an identity or cache key, because the same claims can have many valid strings.
- **C. Time bounds.** `iat` must be 0 or more and `exp` below 2^53, otherwise the token is rejected. `exp - iat` must be at most 300 seconds. Leeway is 30 seconds. Foyr issues 60 seconds.

**Deviations from the spec text (found while planning):**
- The spec says the vectors test regenerates the file and fails if it differs. ECDSA signatures are randomized, so regenerated bytes never match. The test replays every vector through the Go verifier and fails on any mismatch. The key set in the file is deterministic and is compared exactly.
- The spec puts signing in authproxy's `rewrite`. `httputil.ReverseProxy`'s `Rewrite` cannot fail a request, so signing happens in `ServeHTTP` right after authorization. A signing failure answers 500 and forwards nothing; `rewrite` only copies the finished header.

**Why:** ES256 is the one algorithm every JWT library and runtime verifies without extra code, and a signature (not a shared secret) means apps can check identity without being able to forge it. Keeping the key separate from the session key keeps "authproxy cannot create sessions" true and means a session token can never pass for an assertion. Opaque ids stop leaking row counts and keep ids stable if the table ever changes. A `kid` thumbprint plus a published key set lets keys rotate without a deploy that must land at the same moment as another. Signing cost is small: the benchmark measured about 26,000 ns/op on the developer's machine against a 200,000 ns/op budget, so it is done on every request with no cache.

**Consequences:** Everyone with a session older than this change is asked to sign in once. A new authproxy needs `assertion-key` and a new foyrd needs `assertion-public-keys` in SSM before they deploy, or the task fails to start and the circuit breaker rolls back; both are created once by `scripts/platform-secrets.sh` and never overwritten by it. Rotation is the four-step runbook in docs/dev/platform.md. Apps deployed before this change get the new headers but not `FOYR_APP_SLUG` or `FOYR_AUTH_URL` until their next deploy. The `@foyr/auth` SDK is still to do; it starts from the vectors.
