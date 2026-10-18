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
copies the key to your clipboard and opens the page; paste it into the box (a login lasts 24 hours; 5 wrong keys lock your address out for 15 minutes).
Locally: start the `foyrd-founder` preview (`.claude/launch.json`, port 8090, local test key in that file).

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
