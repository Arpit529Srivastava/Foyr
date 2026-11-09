# foyrd and authproxy in AWS (step 8c)

```
GitHub ─► https://api.foyr.online/webhooks/github ─► load balancer ─► foyrd (ECS) ─► RDS Postgres
browser ─► https://<slug>.apps.foyr.online ─► load balancer ─► authproxy (ECS) ─► the app
authproxy ─► every 3 s: foyrd.platform.foyr.internal:9090/internal/routes
```
Decisions: ADR-031 (front door), ADR-032 (this guide).

## Setting it up (once)
1. `tofu apply` of the api zone, then add its 4 NS records for `api` at GoDaddy (`tofu output api_name_servers`).
2. Copy the secrets into SSM (prints only version numbers):
   ```bash
   scripts/platform-secrets.sh
   ```
3. `tofu apply` of the rest: database (about 10 minutes), foyrd, the `api.foyr.online` certificate and rule.
4. Copy the local database before foyrd first starts:
   ```bash
   scripts/db-import.sh
   ```
5. Merge to `main`. **Deploy platform** builds and deploys foyrd, then authproxy.
6. GitHub → Settings → Developer settings → GitHub Apps → foyr-dev → Webhook URL:
   `https://api.foyr.online/webhooks/github` (same secret). Stop the local smee relay.

## Day to day
- New code reaches AWS by merging to `main`. Run **Deploy platform** by hand (Actions tab) after changing settings in `infra/`.
- Logs: CloudWatch `/foyr/platform`, streams `foyrd/...` and `authproxy/...`.
- Webhook deliveries and redeliveries: the App's Advanced tab on GitHub.
- Don't run local foyrd with workers at the same time (ADR-032); use `FOYR_WORKERS=0` locally.

## Founder page (ADR-033)
`https://api.foyr.online/founder` shows apps, deployments, build steps, timings, failure reasons and the audit trail.
```bash
scripts/founder-key.sh
```
copies the key to your clipboard and opens the page; paste it into the box (a login lasts 24 hours; 5 attempts lock your address out for 15 minutes; the key stays on the clipboard for 30 seconds).
Locally: start the `foyrd-founder` preview (`.claude/launch.json`, port 8090, local test key in that file).

## Login (ADR-035, once)
Local walkthrough: docs/dev/login.md. To switch it on in AWS:
1. Create a Resend API key with sending access and save it in `~/.config/foyr/resend-api-key`.
2. `scripts/platform-secrets.sh` (makes `~/.config/foyr/session.{key,pub}` the first time and copies all three keys into SSM).
3. `cd infra && tofu plan -out=tfplan`, review, `tofu apply tfplan` (creates the `auth.`/`app.` certificate, nothing else changes yet).
4. `tofu output platform_hosts_validation_records`: add those CNAME records for `foyr.online` at GoDaddy. Wait until the certificate shows as issued in ACM.
5. Set `platform_hosts_live = true` in `infra/terraform.tfvars`, plan, apply (attaches the certificate, sends `auth.` and `app.` to foyrd).
6. `tofu output platform_hosts_cnames`: add the `auth` and `app` CNAME records at GoDaddy.
7. Merge to `main` (or run **Deploy platform**). Then on the founder page open an app, make it private, invite yourself, and open its URL in a private window.

## Identity keys (ADR-038)
authproxy signs an identity assertion for every request; foyrd publishes the matching public keys at `auth.<PLATFORM_DOMAIN>/.well-known/jwks.json`.

**Creating the pair (once).** `scripts/platform-secrets.sh` makes `~/.config/foyr/assertion.key` (private, for authproxy) and `~/.config/foyr/assertion.pub` the first time, then writes SSM `/foyr/platform/assertion-key` and `/foyr/platform/assertion-public-keys`. It writes these two only if they do not exist, and a failed run never overwrites them, so re-running it cannot undo a rotation. If one local file is missing while its partner exists, it stops instead of making a new pair. Both parameters must exist before authproxy and foyrd deploy, or their tasks do not start.

**How the services read them.** ECS injects the values when a task starts: authproxy gets `FOYR_ASSERTION_KEY`, foyrd gets `FOYR_ASSERTION_PUBLIC_KEYS` (a comma-separated list). Changing SSM does nothing until the service is deployed again.

**Rotating (four steps, each safe on its own).** Run in the repo root with `AWS_PROFILE=foyr AWS_REGION=ap-south-1`. First make the new pair without printing it:
```
umask 077
go run ./cmd/assertionkey > ~/.config/foyr/assertion-new.out
sed -n 's/^key=//p' ~/.config/foyr/assertion-new.out | tr -d '\n' > ~/.config/foyr/assertion-new.key
sed -n 's/^public=//p' ~/.config/foyr/assertion-new.out | tr -d '\n' > ~/.config/foyr/assertion-new.pub
rm ~/.config/foyr/assertion-new.out
```
1. Add the new public key to the list, then deploy foyrd. The key set now has old and new.
   ```
   printf '%s,%s' "$(cat ~/.config/foyr/assertion.pub)" "$(cat ~/.config/foyr/assertion-new.pub)" > ~/.config/foyr/assertion-public-keys.next
   aws ssm put-parameter --name /foyr/platform/assertion-public-keys --type SecureString --overwrite --value "file://$HOME/.config/foyr/assertion-public-keys.next"
   ```
2. Wait at least an hour, so every SDK cache has seen the new set.
3. Replace the private key, then deploy authproxy.
   ```
   aws ssm put-parameter --name /foyr/platform/assertion-key --type SecureString --overwrite --value "file://$HOME/.config/foyr/assertion-new.key"
   ```
4. After at least two minutes (one assertion lifetime plus leeway), remove the old public key, then deploy foyrd.
   ```
   aws ssm put-parameter --name /foyr/platform/assertion-public-keys --type SecureString --overwrite --value "file://$HOME/.config/foyr/assertion-new.pub"
   ```
   Then keep the local copies in step: `mv ~/.config/foyr/assertion-new.key ~/.config/foyr/assertion.key`, the same for `.pub`, and `rm ~/.config/foyr/assertion-public-keys.next`.

"Deploy" means merge to `main` or run **Deploy platform**; if nothing in the image changed, force a new deployment of that ECS service so its tasks start and read SSM again. Skipping the wait in step 2 costs one extra key set request in an SDK, not an outage.

### Rolling out this release (ADR-038)

Order: (a) create the keys with `scripts/platform-secrets.sh`, (b) `tofu apply`, (c) deploy foyrd, (d) deploy authproxy. Do (a) and (b) before merging to `main`: the Deploy platform workflow runs on pushes to `main` that change `cmd/**` or `internal/**`, and would otherwise deploy a new foyrd on a task definition revision without `FOYR_ASSERTION_PUBLIC_KEYS`. That task fails to start and the ECS circuit breaker rolls it back (safe, but a red deploy). The same rollback happens whenever one of the two SSM parameters is missing.

Rollback: roll authproxy back first, then foyrd. Never roll foyrd back past this release while the new authproxy runs: the new authproxy refuses sessions without a public id and answers a "Sign-in unavailable" page (502), so a foyrd rollback alone makes sign-in unavailable.

## Terms

**RDS**: AWS runs Postgres for us: backups, patches, disk.
- Example 1: foyrd's `deployments` and `audit_events` tables now live in `foyr` on RDS.
- Example 2: a restore to any second in the last 7 days is a console click.
- Why: the laptop's Postgres stops when the laptop sleeps.

**Task role vs execution role**: the execution role is what ECS uses to start a container (pull image, read secrets); the task role is what the code inside may do.
- Example 1: the platform execution role reads `/foyr/platform/database-url`; foyrd's code never needs to.
- Example 2: foyrd's task role may start CodeBuild; authproxy has no task role at all.
- Why: each piece gets only the permissions it needs.

**Service discovery (Cloud Map)**: a private phone book of running tasks.
- Example 1: `foyrd.platform.foyr.internal` → foyrd's private IP, updated when it restarts.
- Example 2: `d-4.apps.foyr.internal` → foyr-hello's task.
- Why: task IPs change on every deploy; names don't.

**Host-based routing**: the load balancer picks a target by the hostname asked for.
- Example 1: `api.foyr.online` → foyrd.
- Example 2: `foyr-hello.apps.foyr.online` → authproxy.
- Why: one load balancer (one bill) serves both.

## Code map
| Path | What it does |
|---|---|
| `infra/rds.tf` | database, its security group, the connection string in SSM |
| `infra/foyrd.tf` | foyrd service, task role, security groups, Cloud Map name, `api` rule |
| `infra/dns.tf` | `apps` and `api` zones and certificates |
| `infra/github.tf` | the deploy role |
| `cmd/foyrd/Dockerfile`, `.github/workflows/platform-deploy.yml` | images and deploys |
| `scripts/platform-secrets.sh`, `scripts/db-import.sh` | one-time setup |
| `internal/founder`, `scripts/founder-key.sh` | founder page and its key |

## Pausing to save money
`scripts/aws-pause.sh pause` scales apps, foyrd and authproxy to 0 tasks and stops the database; `scripts/aws-pause.sh resume` brings them back (about 5 minutes for the database). While paused, apps and the founder page are down and pushes are not deployed. AWS restarts a stopped database after 7 days.
