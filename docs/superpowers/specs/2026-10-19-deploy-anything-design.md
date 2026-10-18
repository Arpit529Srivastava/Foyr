# Deploy anything, see everything: product plan

Status: draft for the founder's approval, 2026-10-19.

## Why

Vercel is built for frontends. Backends hit its walls: functions stop after 300 s (800 s on paid plans),
there are no always-on workers, long connections live inside a function's time limit, and the database
comes from someone else. The common advice is "keep the frontend on Vercel, put the backend somewhere else".
The alternatives each have a known weakness: Railway is the fastest to start but had an 8-hour outage in
May 2026; Render's free tier sleeps after 15 minutes and its team pricing keeps changing; Fly.io wants a
Dockerfile and its Postgres is unmanaged.

Foyr wins by being the place where **anything a developer has already written runs, frontend or backend, in
any language, with login, users and roles already built in**, and where the developer can see everything
about their app without opening AWS.

## Who it's for, from their side

A developer with a repo that works on their laptop. They want it online, private to their team or customers,
without writing a Dockerfile, an auth system or a CI pipeline. Their moments:

1. **"Will it work with my code?"** They pick a repo and see, before any build, what Foyr found:
   "Go 1.23 API, listens on PORT, start command `./server`". They can change it in one place.
2. **"Is it live?"** They watch the build stream line by line, then get a URL.
3. **"Why did it break?"** A failure says what happened in plain words and what to do:
   "Your app listened on port 3000, Foyr sends traffic to $PORT (8080). Read PORT from the environment."
   The full log is one click away, never hidden.
4. **"Who can use it?"** People sign in with a magic link; machines (a mobile app, a webhook, another server)
   use an API token. The app gets the same `X-Foyr-User` and role headers either way.
5. **"What is it doing right now?"** Live logs, requests and errors, CPU and memory, on one page.
6. **"Undo."** One click back to the last good deployment.

## Goals

- Any language with no Dockerfile: Node, Python, Go first; Ruby, Java, PHP, Rust, static sites next.
  A Dockerfile in the repo always wins over detection.
- Backends are first-class: always-on, websockets, no request time limit, workers, cron, a release
  command for migrations, Postgres with `DATABASE_URL` set.
- Built-in auth for browsers and machines: magic-link login, invites, roles, API tokens, public paths.
- The customer sees everything about their own product: deployments, full build logs, runtime logs,
  requests and errors, resource use, settings, people with access.
- The founder sees everything about every customer and about Foyr itself (foyrd, authproxy).

## Non-goals (for now)

- An edge network or CDN; a global multi-region footprint. One region (ap-south-1) is the start, and close
  to Indian developers on purpose.
- Serverless functions. Foyr runs containers.
- Self-hosting Foyr.

## Anti-goals (a failure even if it works)

- A customer has to open the AWS console, or learns Foyr runs on ECS, to fix their app.
- A failure shows "Build failed" with no reason or no log.
- A customer can see anything of another customer: logs, names, counts.
- Foyr's internals (foyrd, authproxy, the platform database) show anywhere outside the founder page.
- Secrets appear in logs, pages or the database (values stay in SSM, ADR-032).

## Two dashboards, two audiences

### Customer dashboard (app.foyr.online)

- **Sidebar:** account switcher (personal or team), ⌘K search, "Your apps" with live dots, "New app".
- **New app:** pick a GitHub repo (via the existing GitHub App install), see the detection result,
  edit if needed, deploy. A monorepo can add several services (for example `web/` and `api/`) to one app.
- **Per-app tabs:**
  - **Overview:** production URL, status, last deployment, requests and errors in the last 24 h, quick
    links.
  - **Deployments:** every deployment with commit, who pushed, status, time to live; a detail page with
    steps, the full build log, and Rollback or Redeploy.
  - **Logs:** runtime output, live tail and search, filter by service and deployment.
  - **Metrics:** requests, error rate, response time (from authproxy), CPU and memory (from ECS).
  - **Access:** who can sign in, invites, roles, API tokens, paths that skip login (for example
    `/webhooks/*`, `/health`), whether the whole app is public.
  - **Environment:** variables and secrets, per environment (production, preview later).
  - **Domains:** the Foyr subdomain now; custom domains later.
  - **Settings:** name, branch, root directory, build and start commands, port, health check path,
    release command, CPU and memory, delete app.
- **Account:** members, usage, billing (later).

### Founder page (api.foyr.online/founder, existing)

Everything above for every customer, plus Foyr itself:

- **Users:** every installation and person, their apps, deployments, last push, status; a detail page.
- **All deployments and all logs,** across customers, with filters.
- **Platform:** foyrd and authproxy each show the running image (git sha), task revision, when it went
  out, healthy task count, recent rollouts, and their own logs from `/foyr/platform`.
- **Queue and jobs:** what's waiting, running, retried, failed.
- **Activity:** the audit log (exists).

## What exists today, and the gaps

| Piece | Today | Gap |
|---|---|---|
| Build | CodeBuild: repo Dockerfile, else a Next.js template | Detection for other languages |
| Run | Fargate service per deployment, health check `/api/health` or `/`, blue/green | Port, health path, start command settings; workers; cron; release command |
| Routing | authproxy on `<slug>.apps.foyr.online` | Login check, API tokens, public paths |
| Logs | `/foyr/builds`, `/foyr/apps`, `/foyr/platform` in CloudWatch; read only on failure | Full and live logs in both dashboards |
| Data | installations, apps, deployments, steps, events | users, sessions, members, tokens, env vars, services, app settings |
| Dashboards | Founder page (Postgres data only) | Customer dashboard; founder users, platform and logs pages |

## Decided

- Platform names and domains as in CLAUDE.md; nothing hardcoded (ADR-015, ADR-028).
- Secrets in SSM, injected through task-definition `secrets` (ADR-032).
- Every state change writes its audit event in the same transaction; every tenant row carries
  `installation_id` (ADR-026). New tables in new numbered migrations (005 onwards).
- The founder page stays the only place that shows Foyr's internals.

## My calls (the founder can override)

- **Language detection with Railpack** (Railway's open-source builder, the successor to Nixpacks), run
  inside the existing CodeBuild job: it writes a build plan, BuildKit builds the image. A repo Dockerfile
  still wins. The detection result is saved on the deployment so both dashboards can show it.
- **The customer dashboard is server-rendered Go in foyrd,** like the founder page, not the Next.js `web`
  service in system-design.md. One language, one deploy, the security work already done (nonce CSP,
  sessions, same-origin posts) is reused, and it ships faster. This needs a new ADR replacing that line of
  the design. It can move to Next.js later if the dashboard needs heavy client-side state.
- **Live logs with CloudWatch Logs `StartLiveTail`,** history with `FilterLogEvents`, streamed to the browser
  over server-sent events. foyrd reads only the streams of apps the signed-in user can see.
- **Request metrics from authproxy,** written as CloudWatch embedded-metric log lines per app, so no new
  service is needed. CPU and memory from ECS Container Insights or task metadata.
- **API tokens** are random, shown once, stored hashed (SHA-256), scoped to one app, revocable, with a last
  used time. authproxy accepts `Authorization: Bearer foyr_...` as well as the session cookie.

## Phases

### Phase 1: YC demo (Oct 19 to Nov 1)

Must have, in order:
1. Magic-link login and sessions (already planned for Oct 17 to 23); invites by email.
2. authproxy checks the session, sends `X-Foyr-*` headers; API tokens and public paths for backends.
3. Customer dashboard: sidebar with app switcher, new app from a GitHub repo, Overview, Deployments with
   the full build log, Logs (recent output), Settings (port, health path, start command) and Environment.
4. Language detection for Node, Python and Go (moved out of the cut list, because it is the "deploy
   anything" pitch).
5. Founder page: Users page, Platform page (foyrd, authproxy), full logs on deployment pages.

Stretch: live log tail, Rollback, Access tab with roles.

Demo script: push a FastAPI backend and a Next.js frontend with no Dockerfile, both live in about 2 minutes;
the frontend asks for login, the backend answers a `curl` with an API token; invite a teammate.

### Phase 2: after applying (November)

Postgres per app (`DATABASE_URL`, shared Aurora as designed), release command, workers, cron, websockets
verified, live logs, Metrics tab, Rollback and Redeploy, preview deployments per pull request,
custom domains, ⌘K, roles and `hasPermission`, more languages.

### Phase 3: grow (December onwards)

Teams and billing, usage limits, notifications (email, Slack), deploy hooks, CLI and API, audit log for
customers, scale-to-zero, volumes, private networking between a customer's services.

## Security

- A customer session can read only rows with its own `installation_id`; every query takes it, and a test
  per handler proves another account's app id returns 404.
- Log reads go through foyrd, checked against app access; the browser never gets AWS credentials.
- The dashboard reuses the founder page's headers: nonce CSP, HSTS, frame deny, same-origin posts.
- foyrd's role gets only the new read actions it needs, scoped to Foyr's log groups and clusters.

## Left to the builder

- Exact page layouts within the design tokens already used on the founder page.
- How detection output is shaped and stored, and how it is overridden in Settings.
- The order of migrations and handlers, and the breakdown into PRs.
- How live tail is limited (time, lines, connections) so it can't run up AWS cost.

## Open

- Pricing and plan limits.
- When the separate apps domain is bought (before the first outside user, ADR-028).
