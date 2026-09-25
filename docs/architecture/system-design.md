# Foyr — System Design

Status: **approved** (2026-09-26, domains revised same day — see ADR-015) · Region: **AWS ap-south-1 (Mumbai)**

| Domain | Serves | Hosted on |
|---|---|---|
| `foyr.online` (+ `www`) | Marketing site and public docs | Vercel |
| `app.foyr.online` | Dashboard (**web**) | AWS |
| `api.foyr.online`, `auth.foyr.online` | **foyrd** (API, login) | AWS |
| `<app>.foyrapps.online` | Customer apps via **authproxy** | AWS |
| customer custom domains | Customer apps via **authproxy** | AWS |

In code these are `PLATFORM_DOMAIN` (`foyr.online`) and `APPS_DOMAIN` (`foyrapps.online`). Never hardcode either.

This document describes how Foyr works end to end. Decisions and the reasoning behind
them are recorded in [decisions.md](./decisions.md).

## 1. Big picture

```
                     DEVELOPERS (our customers)               END USERS (their teammates/clients)
                              │                                          │
                              ▼                                          ▼
   ┌──────── Route53: app./api./auth.foyr.online, *.foyrapps.online, customer CNAMEs ──┐
   │                                   ALB (TLS via ACM)                               │
   │ app.foyr.online ─► web     api./auth.foyr.online ──► foyrd   *.foyrapps.online  │
   │                                                                  + custom domains │
   └──────┬──────────────────────────────┬──────────────────────────────────┬──────────┘
          ▼                              ▼                                  ▼
   ┌─────────────┐              ┌─────────────────┐   config poll   ┌──────────────┐
   │ web         │──── API ────►│ foyrd (Go)      │◄────────────────│ authproxy(Go)│
   │ Next.js     │              │ API · login     │                 │ auth check   │
   │ dashboard   │              │ deployer · jobs │                 │ routing      │
   └─────────────┘              └──┬──┬──┬──┬─────┘                 │ blue/green   │
                                   │  │  │  │                       └──────┬───────┘
                 ┌─────────────────┘  │  │  └──────────┐                   │ private network
                 ▼                    ▼  ▼             ▼                   ▼
          ┌────────────┐   ┌──────────┐ ┌─────┐ ┌─────────────┐   ┌──────────────────┐
          │ Postgres   │   │CodeBuild │ │ SES │ │ GitHub App  │   │ Tenant apps      │
          │ (platform) │   │→ ECR     │ │email│ │ webhooks    │   │ ECS Fargate      │
          └────────────┘   └──────────┘ └─────┘ └─────────────┘   │ 1 service / app  │
                                                                  └────────┬─────────┘
                                                                           ▼
                                                                  ┌──────────────────┐
                                                                  │ Shared Aurora    │
                                                                  │ 1 DB per app     │
                                                                  └──────────────────┘
```

Three deployables (**web**, **foyrd**, **authproxy**) and one platform database.
Everything else is a managed AWS service. This is deliberately *not* a microservice
architecture — see [ADR-005](./decisions.md#adr-005-three-deployables-not-microservices).

> The marketing site and public docs (`foyr.online` landing + `/docs`) are hosted
> separately on Vercel. The diagram above covers the product platform. Customer apps live on
> a **separate registrable domain** (`foyrapps.online`) so they can never set cookies on, or be
> treated as same-site with, the dashboard and auth — see [ADR-015](./decisions.md#adr-015-customer-apps-on-a-separate-domain).

## 2. Components

| Component | Responsibility | Scaling |
|---|---|---|
| **web** | Next.js dashboard: apps, deploy, build logs, env vars, members & roles, domains | 1–2 Fargate tasks |
| **foyrd** | Go control plane: REST API, developer login (GitHub), end-user login (magic link), invites, deploy orchestration via a Postgres job queue, GitHub webhooks, config feed for the proxy | 2+ stateless replicas; jobs claimed with `FOR UPDATE SKIP LOCKED` |
| **authproxy** | Go gatekeeper on every tenant request: verifies session, enforces access, injects identity headers, routes to the live deployment, (later) wakes sleeping apps | 2–N stateless replicas, autoscaled. **Never calls foyrd or the DB per request** — serves from an in-memory config cache |
| **CodeBuild** | Clone → build image → push to ECR. One isolated machine per build | Managed, per-minute |
| **Tenant apps** | Customer containers, one ECS service per deployment. Reachable **only** from authproxy | Per-app autoscaling |
| **Platform Postgres** | Foyr's own data | RDS `db.t4g.micro` to start |

## 3. Two kinds of users

- **Developers** (Foyr customers) sign in with **GitHub**. They connect repos, deploy and manage apps.
- **End users** (people a developer invites) sign in with a **magic link** by email. Access is
  **per app**: an invite to app A grants nothing on app B. No GitHub account needed.

The same email can be both; it maps to a single `users` row.

## 4. Key flows

### 4a. Connecting a repo

Foyr uses a **GitHub App** (not a plain OAuth app):

1. Developer signs in with GitHub (GitHub App user authorization).
2. Developer installs the Foyr GitHub App on selected repos only (read-only contents).
3. Foyr receives **push webhooks** (this is how `git push` deploys) and mints **short-lived
   installation tokens** for cloning. No long-lived GitHub secrets are stored.

### 4b. Deploy: `git push` → live

```
push ─► GitHub webhook ─► foyrd: create Deployment(status=queued) + Job
job worker ─► start CodeBuild(repo, commit, installation token)
   CodeBuild: clone → detect (repo Dockerfile? else Next.js template) → docker build → push ECR:<app>:<sha>
   logs ─► CloudWatch ─► streamed to dashboard
foyrd ─► register ECS task definition (image, env from SSM, cpu/mem)
      ─► create ECS service "green" for this deployment (Cloud Map name d-<id>.apps.foyr.internal)
      ─► wait for health check (GET /health or / → 2xx within timeout)
   healthy?  yes ─► flip proxy config: app → green; drain blue after 5 min; status=live
             no  ─► delete green; blue keeps serving; status=failed (+ logs)
```

**Blue/green lives in the proxy**, not in ALB target groups. foyrd changes which
deployment the app points to; authproxy picks it up on its next config poll. Benefits:
no per-app ALB rules or target groups (both have hard quotas), and rollback is an
instant pointer flip.

Service discovery: authproxy reaches tenant tasks through **AWS Cloud Map** private DNS.

### 4c. End user opens a protected app

```
1. GET https://acme-crm.foyrapps.online/reports          (no cookie)
2. authproxy ─► 302 https://auth.foyr.online/login?app=acme-crm&return=/reports
3. user enters email ─► foyrd: is this email a member/invitee of this app?
                     ─► SES sends magic link (15 min, single use)
4. click link ─► foyrd verifies ─► 302 https://acme-crm.foyrapps.online/__foyr/callback?code=…
5. authproxy exchanges code (single use, 60 s) ─► sets __foyr_session
   (host-only, HttpOnly, Secure, SameSite=Lax)
6. each subsequent request:
   authproxy verifies session JWT locally (public key) + looks up role in config cache
   ─► strips any inbound X-Foyr-* headers (anti-spoofing)
   ─► adds X-Foyr-User-Id, X-Foyr-Email, X-Foyr-Role, X-Foyr-Assertion (short-lived signed JWT)
   ─► forwards to the live deployment
```

The per-host callback (steps 4–5) gives every app host its own host-only cookie. That is what
makes **custom domains** work (a cookie on `foyr.online` can't be read on `crm.acme.com`), and it
keeps apps isolated from each other even on the shared apps domain.

**Access modes** per app:

| Mode | Behaviour |
|---|---|
| `private` (default) | Members only. Everyone else is sent to login |
| `public` | Anyone can view; identity headers are added if the visitor is signed in |

### 4d. Invites & roles

- Owner enters emails + role → `invites` row → SES email → first login converts it to an `app_members` row.
- Fixed roles: `owner > admin > member > viewer`.
- Apps can map actions to minimum roles in an optional `foyr.json`:
  ```json
  { "permissions": { "reports:export": "member", "billing:edit": "admin" } }
  ```
- **Role changes and removals apply within ~5 s**: the proxy reads roles from its config
  cache, not from the session token.

### 4e. Developer experience (SDK)

Without any SDK, an app can read the `X-Foyr-*` headers directly. With `@foyr/auth`
(and later `foyr-auth` for Python):

```ts
import { getUser, hasPermission } from "@foyr/auth";

const user = getUser(req);          // verifies X-Foyr-Assertion → { id, email, role } | null
if (!hasPermission(user, "reports:export")) return res.status(403).end();
```

The SDK is a thin verifier (<100 lines per language). **Nothing is injected into the user's build.**

### 4f. Custom domains (Phase 3)

Developer adds `crm.acme.com` → Foyr shows a CNAME to `acme-crm.foyrapps.online` plus an ACM
validation record → ACM issues the certificate → attached to the ALB via SNI → hostname
added to the proxy routing table.

Known limit: an ALB holds ~25 certificates by default (raisable to ~100). Beyond that,
authproxy terminates TLS itself with on-demand certificates (**CertMagic**) behind an NLB.
The design allows this because the proxy already owns routing.

### 4g. Sleeping apps / scale-to-zero (designed now, built in Phase 2–3)

Each app has `sleep_after` (e.g. 15 min). The proxy tracks last-request time per app; an
idle checker in foyrd sets ECS `desiredCount=0`. The next request is held by the proxy,
which shows a "Waking up…" page, triggers a wake, and forwards once healthy (~20–40 s).

## 5. Data model (platform Postgres)

```
users          id, email, github_id?, name, created_at
teams          id, name, plan
team_members   team_id, user_id, role
apps           id, team_id, slug, repo, branch, access_mode, live_deployment_id,
               sleep_after, cpu, memory, created_at
deployments    id, app_id, commit_sha, image_uri,
               status (queued|building|deploying|live|failed|superseded),
               build_id, ecs_service, started_at, finished_at, error
app_members    app_id, user_id, role (owner|admin|member|viewer)
invites        id, app_id, email, role, token_hash, expires_at, accepted_at
env_vars       app_id, key, ssm_param_arn        -- values live in SSM, never in Postgres
domains        id, app_id, hostname, status, acm_cert_arn
login_tokens   token_hash, user_id, app_id?, expires_at, used_at
sessions       id, user_id, app_id?, expires_at, revoked_at
jobs           id, kind, payload jsonb, status, run_after, attempts, locked_by, locked_at
audit_log      id, actor_id, app_id, action, data jsonb, at
```

## 6. AWS layout (ap-south-1)

- **Region:** `ap-south-1` (Mumbai), 2 availability zones.
- **VPC:** public subnets (ALB, NAT) and private subnets. Tenant tasks run in **dedicated
  private subnets**, separate from platform services.
- **Egress:** a **fck-nat** instance (`t4g.nano`, ~$4/mo) instead of a NAT Gateway during the
  build phase; move to NAT Gateway once there are paying users. Free S3 gateway endpoint.
- **ECS:** one Fargate cluster. Tenant apps may use **Fargate Spot** for free tiers.
- **Secrets / env vars:** SSM Parameter Store `SecureString`, injected via task-definition `secrets`.
- **Tenant databases:** **one shared Aurora Serverless v2 cluster**, with a separate database
  and login per app; connection string injected as `DATABASE_URL`. Dedicated clusters on paid plans later.
- **Email:** SES from `mail.foyr.online` with SPF, DKIM and DMARC.
- **Infrastructure as code:** OpenTofu manages *static* infrastructure (VPC, ALB, cluster, RDS,
  ECR, IAM, CodeBuild project). foyrd creates *per-app* resources at runtime with the AWS SDK
  for Go v2, and tags everything `foyr:app=<id>` for cleanup.

## 7. Security & tenant isolation

- Tenant security groups allow inbound **only from the authproxy security group** on the app
  port. Tenants cannot reach each other, foyrd or the platform database; NACLs on tenant
  subnets deny traffic to platform subnets.
- Tenant tasks run with **no IAM task role** (no AWS credentials), CPU/memory limits, and a
  read-only root filesystem where possible.
- CodeBuild's role is scoped to pushing to that app's ECR repository; clone tokens expire in 1 h.
- authproxy strips spoofed `X-Foyr-*` headers; apps can verify `X-Foyr-Assertion` against
  Foyr's published public keys (JWKS).
- Customer apps are served from `APPS_DOMAIN`, a different registrable domain from the dashboard and
  auth (`PLATFORM_DOMAIN`), so a malicious app cannot set cookies on `.foyr.online` or ride on
  same-site requests to the dashboard. Submit `foyrapps.online` to the Public Suffix List before
  public launch so browsers also isolate apps from each other.
- App slugs are validated (`[a-z0-9-]{3,40}`) and a reserved list is enforced: `app`, `api`, `auth`,
  `www`, `docs`, `admin`, `status`, `mail`, `static`, `cdn`, `foyr`.
- Magic-link and invite tokens are stored hashed and are single use; login endpoints are rate limited.
- Session tokens: ES256 JWTs, 12 h expiry, signing key in KMS/SSM with rotation.

## 8. Failure handling

| Failure | Behaviour |
|---|---|
| foyrd down | **Apps keep serving** — proxy uses its last config; new logins pause |
| A proxy replica dies | ALB routes around it (2+ replicas across AZs) |
| Build fails | Deployment marked failed with logs; previous version stays live |
| New version fails health check | Green torn down automatically; blue keeps serving |
| Tenant app crashes | ECS restarts it; proxy returns a Foyr-branded 502 with a "check logs" hint |
| Jobs stuck | `locked_at` timeout → retry with backoff → failed after N attempts |

## 9. Observability

- **Tenant logs:** `awslogs` driver → one CloudWatch log group per app → dashboard viewer (Live Tail for streaming).
- **Metrics:** ECS CPU/memory from CloudWatch; **request counts per app from authproxy**
  (also the metering source for usage billing).
- **Platform:** Sentry (foyrd, authproxy, web) and PostHog (dashboard).

## 10. Repository layout

```
foyr/
  cmd/foyrd/  cmd/authproxy/        # Go entrypoints (single go.mod)
  internal/{auth,apps,deploy,build,proxy,jobs,aws,github,db,email}
  web/                              # Next.js dashboard
  site/                             # marketing site + public docs (Vercel)
  sdk/js/                           # @foyr/auth
  templates/nextjs/Dockerfile       # build template
  infra/                            # OpenTofu
  db/migrations/
  docs/                             # internal + public documentation source
```

## 11. Estimated monthly cost during build (from AWS credits)

Mumbai pricing is within ~5–10% of us-east-1.

| Item | ~USD / month |
|---|---|
| ALB | 20–25 |
| fck-nat instance | 4 |
| Platform Fargate (web, foyrd, 2× authproxy, small) | 35–50 |
| RDS `db.t4g.micro` | 15 |
| Test tenant apps (5 × 0.25 vCPU / 0.5 GB) | 45–50 |
| CodeBuild, ECR, CloudWatch, Route53, SES | 10–20 |
| Aurora (Phase 3+, can pause to 0) | 0–40 |
| **Total** | **~$130–200** |
