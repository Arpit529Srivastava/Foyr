# How requests reach an app (step 8)

```
browser ─► https://foyr-hello.apps.foyr.online
             ─► load balancer (HTTPS)                     8b
             ─► authproxy: "foyr-hello" → d-4             8a, this guide
             ─► d-4.apps.foyr.internal:3000 (the live task, private network only)
authproxy ◄── every 3 s: foyrd :9090/internal/routes (bearer token) ── keeps the last good table on errors
```
Decision: ADR-030.

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
