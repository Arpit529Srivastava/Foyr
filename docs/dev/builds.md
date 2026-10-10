# How builds work (step 6b)

A push becomes a queued deployment (docs/dev/database.md). A **worker** inside foyrd picks it up,
runs it on **CodeBuild**, records every step, and ends with an image in **ECR** or a clear failure.
Decision: ADR-027.

```
 Postgres                foyrd worker                         AWS
 ────────                ────────────                         ───
 job (queued) ──claim──► 1. deployment → "building"
                         2. GitHub: 1-hour read-only token
                         3. token → SSM ────────────────────► /foyr/builds/d-<id>/token (encrypted)
                         4. ECR repo apps/<app> exists? ────► create (immutable, scan, keep 10)
                         5. start CodeBuild ────────────────► clone (token in header)
                                                              Dockerfile? else Next.js template
                                                              docker build, docker push
                         6. every 10 s: phases → steps ◄───── phase times + status + log link
                            (token deleted once clone is over)
                         7. "built" + image, or "failed" + step + reason + last 30 log lines
 job (done) ◄──────────
```

## Running it locally
```bash
aws sso login --profile foyr
```
```bash
scripts/foyrd-dev.sh
```
Push to foyr-hello's `main`. Logs show `deployment queued`, `job started`, `build started`,
then `build succeeded` (or `build failed`) and `job done`. To only record pushes without building,
set `FOYR_WORKERS=0`. A build of foyr-hello costs about 2 to 3 US cents (CodeBuild MEDIUM, ~2 to 3 min).

## Terms

**Worker**: a loop in foyrd that takes jobs from the queue and runs them.
- Example 1: 2 workers each ask Postgres for a job every 2 s when idle.
- Example 2: kitchen staff taking the next order ticket from the rail.
- Why: webhooks must answer GitHub in milliseconds; builds take minutes.

**Heartbeat**: the worker renews its 2-minute lease every 30 s while a job runs.
- Example 1: a 4-minute build keeps its lease the whole time.
- Example 2: topping up a parking meter.
- Why: if foyrd dies, renewals stop, the lease expires and another worker takes over.

**Resume**: the CodeBuild ID is saved as soon as a build starts.
- Example 1: foyrd restarts mid-build; the next worker sees the ID and keeps watching that build.
- Example 2: a relay runner takes the baton instead of restarting the race.
- Why: no double builds, no double charges.

**CodeBuild phases → steps**:

| Step (what developers see) | CodeBuild phases |
|---|---|
| Prepare | SUBMITTED, QUEUED, PROVISIONING, DOWNLOAD_SOURCE, INSTALL |
| Clone | PRE_BUILD (ECR login + git fetch) |
| Build image | BUILD (pick Dockerfile or template, docker build) |
| Push image | POST_BUILD (docker push) |

Steps after a failure show as **skipped**. Reading phases from CodeBuild is more reliable than
parsing log text, which the customer's own build output could imitate.

**SSM Parameter Store SecureString**: encrypted storage for one build's token.
- Example 1: `/foyr/builds/d-2/token`, readable only by CodeBuild's role, deleted after the clone.
- Example 2: a hotel safe emptied at checkout.
- Why: CodeBuild masks `PARAMETER_STORE` values in logs; a plain variable would show in the console.

**`http.extraHeader`**: git sends the token as a header.
- Example 1: `git -c http.extraHeader="Authorization: basic …" fetch …`; the URL has no secret.
- Example 2: showing ID at the door instead of writing your password on the envelope.
- Why: tokens in URLs end up in `.git/config` (and so in the image) and in error messages.

**Retry vs fail**:
- Our side (AWS throttling, network): retried after 30 s, then 2 min; after 3 attempts the deployment fails with "this is on our side".
- Their side (build error, timeout after 20 min, no Dockerfile, App removed): failed at once with the step, the reason, the last 30 log lines and the log link.

## What gets built
1. A `Dockerfile` at the repo root: used as is. Any language.
2. Otherwise, `package.json` mentioning `next`: `templates/nextjs/Dockerfile` (Node 22, npm/pnpm/yarn from the lockfile, `next start` on `PORT` 3000). OpenTofu embeds it into the buildspec, so changing the template needs a `tofu apply`.
3. Otherwise: fails with "Add a Dockerfile to the repository root; any language works."

Images: `apps/<app id>:d<deployment id>-<first 12 of commit>`. Tags are immutable and unique per deployment.

## Debugging a build
```sql
SELECT id, status, failure_step, left(failure_reason, 300), log_url FROM deployments ORDER BY id DESC LIMIT 5;
```
```sql
SELECT position, name, status, finished_at - started_at AS took, message FROM deployment_steps
WHERE deployment_id = <id> ORDER BY position;
```
```sql
SELECT id, status, attempts, last_error, run_after FROM jobs ORDER BY id DESC LIMIT 5;
```
Open `log_url` for the full CodeBuild log (CloudWatch, kept 30 days). Search foyrd's logs for the
deployment's `trace_id` to see every line about it.

## Code map
| Path | What it does |
|---|---|
| `internal/worker/worker.go` | the pool: claim, heartbeat, complete, retry with backoff, release on shutdown, give up |
| `internal/build/runner.go` | one build: token, image repo, start, watch, finish, resume |
| `internal/build/steps.go` | CodeBuild phases → steps |
| `internal/awsx/awsx.go` | CodeBuild, ECR, SSM and CloudWatch Logs with AWS SDK v2 |
| `infra/buildspec.yml.tftpl` | what runs on the build machine |
| `templates/nextjs/` | the Next.js Dockerfile and dockerignore |
| `cmd/foyrd/main.go` | `newPool`: AWS credentials check and wiring |
