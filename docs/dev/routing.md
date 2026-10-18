# How requests reach an app (step 8)

```
browser ─► https://foyr-hello.apps.foyr.online
             ─► load balancer (HTTPS)                     8b
             ─► authproxy: "foyr-hello" → d-4             8a, this guide
             ─► d-4.apps.foyr.internal:3000 (the live task, private network only)
authproxy ◄── every 3 s: foyrd :9090/internal/routes (bearer token) ── keeps the last good table on errors
```
Decisions: ADR-030 (routing), ADR-031 (load balancer, certificate, DNS, image pipeline).

## In AWS (8b)
- `https://<slug>.apps.foyr.online` → ALB `foyr-apps` → ECS service `authproxy` (platform subnets) → the app.
- New authproxy code reaches AWS by merging to `main`: the **Deploy platform** workflow builds and deploys it.
  Run it by hand from the Actions tab (Run workflow) after changing authproxy settings in `infra/`.
- Routes come from foyrd in AWS at `foyrd.platform.foyr.internal:9090` (8c, docs/dev/platform.md).
- Logs: CloudWatch `/foyr/platform`, streams `authproxy/...`.

```bash
curl -sI https://foyr-hello.apps.foyr.online/
```

## Running it locally
```bash
scripts/foyrd-dev.sh
```
```bash
scripts/authproxy-dev.sh
```
Then `curl -H "Host: foyr-hello.apps.localhost" localhost:8081/`. Apps run in AWS's private network,
so locally this answers 502 ("This app isn't responding"), which proves the route was found.
`nope.apps.localhost` answers 404. To see a real page, run an app on your Mac and start authproxy with
`FOYR_STATIC_ROUTES=foyr-hello=localhost:3000`, then open http://foyr-hello.apps.localhost:8081.

## Terms

**Slug**: the short name in an app's address.
- Example 1: repo `octo/Foyr_Hello` → `foyr-hello` → `foyr-hello.apps.foyr.online`.
- Example 2: a second `foyr-hello` from another account becomes `foyr-hello-2`; a repo called `admin` becomes `admin-app`.
- Why: hostnames allow only a few characters, must be unique, and must never look like Foyr's own (`api`, `auth`, ...).

**Reverse proxy**: a server that receives the request and forwards it to the real app.
- Example 1: authproxy receives `GET /` for `foyr-hello` and asks `d-4` for it, then returns the answer.
- Example 2: a hotel front desk: guests talk to the desk, the desk calls the right room.
- Why: apps stay private; one front door can add login (Phase 1), switch versions and show friendly errors.

**Polling with a last good copy**: ask for the latest list every few seconds; if asking fails, keep using the old list.
- Example 1: foyrd restarts; for those seconds authproxy keeps routing with the table it already has.
- Example 2: a printed bus timetable still works when the website is down.
- Why: the front door must not depend on the control plane being up.

**Header stripping**: deleting headers that only Foyr is allowed to set.
- Example 1: a visitor sends `X-Foyr-Role: owner`; authproxy deletes it before the app sees the request.
- Example 2: a bank teller ignores a customer's note saying "approved by the manager".
- Why: in Phase 1 apps trust `X-Foyr-*` for identity, so visitors must never be able to forge them.

**Load balancer (ALB)**: AWS's public entry point that receives requests and hands them on.
- Example 1: every `*.apps.foyr.online` request arrives at `foyr-apps` first.
- Example 2: a hotel reception desk: one door for all guests.
- Why: apps and authproxy have no public address; something must face the internet and hold the certificate.

**Wildcard certificate**: one TLS certificate valid for every name at one level, `*.apps.foyr.online`.
- Example 1: covers `foyr-hello.apps.foyr.online` and `todo.apps.foyr.online`.
- Example 2: does not cover `apps.foyr.online` itself or `a.b.apps.foyr.online`.
- Why: the padlock for every new app without issuing a certificate per app.

**DNS delegation**: telling the parent zone "another server answers for this subdomain" with NS records.
- Example 1: GoDaddy holds `foyr.online`; 4 NS records for `apps` send those lookups to Route 53.
- Example 2: a receptionist forwarding all calls for one department to that department's own phone.
- Why: AWS can then create the certificate check and load balancer records itself.

**OIDC deploy role**: GitHub proves to AWS which repo and branch a workflow runs on, and gets a 1-hour key.
- Example 1: a workflow on `main` of Arpit529Srivastava/Foyr can push the authproxy image.
- Example 2: a pull request, or a fork, gets nothing.
- Why: no long-lived AWS keys stored in GitHub that could leak.

## Code map
| Path | What it does |
|---|---|
| `internal/slug` | slug rules, reserved words, candidates |
| `internal/proxy/handler.go` | host → slug → upstream, header rules, error pages, health path |
| `internal/proxy/routes.go` | the table and the poller |
| `internal/proxy/config.go` | authproxy settings |
| `cmd/authproxy/main.go` | wiring |
| `cmd/foyrd/internal.go` | `/internal/routes` on the private listener |
| `internal/db/migrations/004_slugs.sql` | `apps.slug` and backfill |
| `infra/dns.tf`, `infra/alb.tf`, `infra/authproxy.tf` | zone, certificate, load balancer, authproxy service, security groups |
| `infra/github.tf` | OIDC provider and the deploy role |
| `cmd/authproxy/Dockerfile`, `.github/workflows/platform-deploy.yml` | image and deploy pipeline |
