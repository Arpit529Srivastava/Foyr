# The platform database (step 6a)

foyrd keeps its facts in Postgres: which customers installed the GitHub App, which repos are apps,
every deployment, the job queue, every webhook GitHub sent, and an audit trail of who did what.
Large or high-volume data (build logs, app logs, request logs) lives in CloudWatch and S3, never here.
Decisions: ADR-024 (deployment visibility), ADR-025 (step 6 choices), ADR-026 (audit trail and scale).

```
GitHub push ─► foyrd /webhooks/github
                 │  one transaction:
                 ├─► installations   (upsert the customer)
                 ├─► apps            (upsert the repo)
                 ├─► deployments     (status = queued)
                 ├─► deployment_events (timeline: "Queued from push by octo-dev")
                 ├─► jobs            (kind = build, waiting for a worker)
                 └─► audit_events    (deployment.created, actor = the pusher)
               after the response is decided:
                 └─► webhook_deliveries (result ok / ignored / error, how long it took)
```

## Local setup (once)
```bash
brew install postgresql@17
```
```bash
brew services start postgresql@17
```
```bash
echo 'export PATH="/opt/homebrew/opt/postgresql@17/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
```
```bash
createdb foyr_dev && createdb foyr_test
```
`foyr_dev` is what `scripts/foyrd-dev.sh` uses (it sets `FOYR_DATABASE_URL` for you).
`foyr_test` is for tests. Add this to `~/.zshrc` so `go test` uses it:
```bash
export FOYR_TEST_DATABASE_URL="postgres://$USER@localhost:5432/foyr_test?sslmode=disable"
```
Without it, database tests are skipped locally. In CI they always run (a Postgres 17 service in Go CI).

To stop Postgres and save battery: `brew services stop postgresql@17`.

## Settings
| Variable | Required | Notes |
|---|---|---|
| `FOYR_DATABASE_URL` | yes | `postgres://...`; the dev script defaults to local `foyr_dev`. In AWS it comes from SSM `/foyr/platform/` |
| `FOYR_TEST_DATABASE_URL` | for tests | each test gets its own throwaway schema inside this database |

## Terms

**Migration**: a numbered SQL file that changes the database shape, applied once and recorded.
- Example 1: `internal/db/migrations/001_init.sql` creates all tables. A later `002_steps_logs.sql` might add a column.
- Example 2: like app updates on your phone; each runs once, in order, and the phone remembers which ones it has.
- Why: every copy of foyrd (laptop, CI, AWS) ends up with the same tables. foyrd applies new ones at startup, and the table `schema_migrations` remembers what ran.

**Advisory lock**: a named lock in Postgres that programs agree to respect.
- Example 1: two foyrd copies start at once; only the one holding lock `"foyr"` runs migrations, the other waits and then finds nothing to do.
- Example 2: like a single key to a shared room; whoever holds it goes in, the other waits outside.
- Why: without it, two copies could run the same migration at the same moment and fail.

**Transaction**: a group of writes that all happen, or none do.
- Example 1: a push writes a deployment, a job and an audit event. If the job insert fails, the deployment and audit event are rolled back too.
- Example 2: a bank transfer: money leaves one account and arrives in the other, never only half.
- Why: the audit trail can never miss an action, and there is never a deployment without its job.

**Idempotent**: doing it twice has the same effect as doing it once.
- Example 1: GitHub redelivers the same push; `deployments_one_per_commit` (a unique index) means only one deployment exists for that commit.
- Example 2: pressing a lift button five times still calls one lift.
- Why: webhooks can arrive twice (network retries, manual "Redeliver"); we must not build twice.

**Job queue with `FOR UPDATE SKIP LOCKED`**: workers take the next job and skip rows another worker already holds.
- Example 1: two workers poll; worker A locks job 7, worker B skips it and takes job 8. No job runs twice.
- Example 2: supermarket tills; each customer goes to the next free till instead of everyone queuing at till 1.
- Why: we can run as many workers as we like on one Postgres, no extra queue service.

**Lease**: a job is "yours" only until a deadline (`lease_until`); long jobs extend it.
- Example 1: a worker crashes mid-build; after its lease expires another worker picks the job up (attempt 2).
- Example 2: a library book: if you don't renew it by the due date, someone else can borrow it.
- Why: a dead worker can never block a deployment forever. A stale worker that comes back cannot finish the job either (`ErrLeaseLost`).

**Retry with backoff**: a failed job waits a while, then runs again, up to `max_attempts` (3).
- Example 1: AWS says "throttled"; the job is retried in 30 s.
- Example 2: the customer's `npm run build` fails; that is their code, so it is **not** retried, it fails at once with the reason.
- Why: temporary platform errors heal themselves; customer errors need a clear message, not three identical failures.

**Fairness limit**: no installation (customer) runs more than 2 jobs at once (`DefaultMaxRunningPerTenant`).
- Example 1: customer A pushes 50 commits; 2 build, 48 wait. Customer B pushes once and builds straight away.
- Example 2: an all-you-can-eat buffet with "one plate at a time" so one guest can't empty every tray.
- Why: one busy customer must never slow everyone else down.

**Tenant key**: the column that says which customer owns a row (`installation_id` now, `team_id` later).
- Example 1: every deployment, job, audit event and delivery has `installation_id`, and its indexes start with it.
- Example 2: like a flat number on every parcel in a building's mailroom.
- Why: fast per-customer queries, strict separation, and later we can split customers across databases.

**Partitioning**: one big table stored as many smaller tables, here one per month (`audit_events_y2026m10`, ...).
- Example 1: "show this month's audit log" reads only October's partition, not years of rows.
- Example 2: a filing cabinet with a drawer per month; throwing out 2025 means removing a drawer, not pulling out every page.
- Why: the audit log grows forever; partitions keep queries fast and retention cheap. foyrd creates the next months' partitions at startup and daily. `audit_events_default` catches anything outside them so a write never fails.

**Append-only**: rows can be added but never changed or deleted.
- Example 1: `UPDATE audit_events ...` fails with "audit_events is append-only" (a database trigger).
- Example 2: a bank statement; corrections are new lines, never edits to old ones.
- Why: an audit trail anyone can edit proves nothing.

**Trace ID**: one ID that follows a push through every stage.
- Example 1: the webhook request ID `3fa9c1d2e4b5a6f7` is stored on the deployment, its job, its audit events and the delivery row, and it is in every log line.
- Example 2: a courier tracking number that shows up at every depot.
- Why: when something breaks, one search finds the whole story.

**Cursor pagination**: "give me 20 older than the last one I saw" instead of "page 37".
- Example 1: `ListDeployments(app, beforeID=120, 20)` returns deployments 119 and down.
- Example 2: a bookmark in a book instead of counting pages from the start.
- Why: speed stays the same on page 1 and page 10,000.

## Tables
| Table | One row is | Kept |
|---|---|---|
| `installations` | a GitHub account that installed the App | forever (marked `removed_at` on uninstall) |
| `apps` | a repo connected to Foyr | forever (marked `removed_at`) |
| `deployments` | one push of one commit: who pushed, commit, message, trace ID, status, failed step and reason, times | forever |
| `deployment_steps` | one step of a deployment (clone, build, push...) with times and a log link | with the deployment (filled from 6b) |
| `deployment_events` | one line of a deployment's timeline | with the deployment |
| `jobs` | background work for a worker | 7 days after finishing |
| `webhook_deliveries` | one webhook GitHub sent us: result, detail, duration, attempts | 30 days |
| `audit_events` | who did what, when (append-only, monthly partitions) | forever for now; per-plan retention later (ADR-026) |

Deployment statuses: `queued → building → built` or `failed`, and `canceled`. Anything else is
refused (`ErrBadTransition`). Step 6b and later add release statuses.

## Looking at the data
```bash
psql foyr_dev
```
Recent deployments:
```sql
SELECT id, status, branch, left(commit_sha, 7) AS commit, pusher_login, failure_step, created_at
FROM deployments ORDER BY id DESC LIMIT 10;
```
A deployment's timeline:
```sql
SELECT occurred_at, from_status, to_status, message FROM deployment_events WHERE deployment_id = 1 ORDER BY id;
```
Everything that happened for one customer:
```sql
SELECT occurred_at, actor_login, action, target_type, target_id, details
FROM audit_events WHERE installation_id = 169884990 ORDER BY occurred_at DESC LIMIT 20;
```
Did GitHub tell us about my push?
```sql
SELECT received_at, event, result, detail, duration_ms, attempts FROM webhook_deliveries ORDER BY received_at DESC LIMIT 10;
```
Follow one trace everywhere:
```sql
SELECT 'deployment' AS kind, id::text FROM deployments WHERE trace_id = '<id>'
UNION ALL SELECT 'job', id::text FROM jobs WHERE trace_id = '<id>'
UNION ALL SELECT 'audit', action FROM audit_events WHERE trace_id = '<id>';
```
The job queue:
```sql
SELECT id, status, attempts, run_after, locked_by, lease_until, last_error FROM jobs ORDER BY id DESC LIMIT 10;
```

## Code map
| Path | What it does |
|---|---|
| `internal/db/db.go` | `Open` the pool, `Migrate` with the advisory lock |
| `internal/db/migrations/*.sql` | the schema, embedded into the foyrd binary |
| `internal/db/dbtest` | gives each test its own migrated schema |
| `internal/store/deployments.go` | `CreateDeployment`, `SetDeploymentStatus`, `ListDeployments`, `Timeline` |
| `internal/store/installations.go` | `SyncInstallation` for install, uninstall, suspend, repos added or removed |
| `internal/store/jobs.go` | `ClaimJob` (fair, leased), `ExtendLease`, `CompleteJob`, `FailJob` |
| `internal/store/maintenance.go` | `RecordDelivery`, `EnsureAuditPartitions`, `Prune` |
| `internal/store/store.go` | the audit helper and `ListAudit` |
| `cmd/foyrd/server.go` | webhook → store |
| `cmd/foyrd/maintenance.go` | daily partitions and cleanup |

## Adding a migration
1. Create `internal/db/migrations/00N_short_name.sql` (next number). Never edit a migration that is merged.
2. Add or update tests in `internal/store`.
3. `go test ./internal/...` with `FOYR_TEST_DATABASE_URL` set; restart foyrd and it applies the file.
