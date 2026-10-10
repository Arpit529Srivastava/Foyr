# How deploys work (step 7)

After a build, the image runs on ECS Fargate and becomes the app's **live** version once its health
check passes (ADR-029). Builds are in docs/dev/builds.md.

```
build done ─► deploy job ─► task definition app-<app> (image, PORT=3000, health check, no AWS role)
                         ─► ECS service d-<deployment> (1 Spot task, tenant subnet, no public IP)
                         ─► Cloud Map name d-<deployment>.apps.foyr.internal (for authproxy, step 8)
           every 5 s: task RUNNING? ─► step "Start" done
                      task HEALTHY? ─► step "Health check" done ─► live
                                        previous live ─► superseded ─► retire job in 5 min
                      task STOPPED or 6 min passed ─► failed (+ reason, exit code, last 30 log lines)
                                        the new service is deleted; the previous live one keeps serving
```

## Running it locally
```bash
aws sso login --profile foyr
```
```bash
scripts/infra-env.sh
```
That writes the runtime settings (subnets, security group, roles, namespace, log group) to
`~/.config/foyr/infra.env`; run it again after infra changes. Then:
```bash
scripts/foyrd-dev.sh
```
Push to foyr-hello. Logs go `build succeeded`, `service started`, `deployment live`.

## Terms

**Task definition**: the recipe for running a container: image, memory, port, environment, health check.
- Example 1: `app-1:3` is revision 3 of app 1's recipe, pointing at image `apps/1:d3-…`.
- Example 2: a recipe card: same dish, new revision each time an ingredient changes.
- Why: ECS can only start what a task definition describes; old revisions stay for rollback.

**ECS service**: keeps N copies of a task running and replaces them if they die.
- Example 1: `d-3` keeps one task of `app-1:3` alive; if the Spot machine is reclaimed, ECS starts another.
- Example 2: a shift manager who always keeps one person at the counter.
- Why: a bare task that crashes stays dead; a service heals it.

**Fargate Spot**: spare AWS capacity, about 70% cheaper, that AWS can take back with 2 minutes' notice.
- Example 1: foyr-hello runs on Spot for a few dollars a month instead of about ten.
- Example 2: standby airline seats: cheap, occasionally you're moved to the next flight.
- Why: fine for early apps, because the service restarts the task. Paid plans can use `FARGATE`.

**Container health check**: a command ECS runs inside the container every 10 s.
- Example 1: `wget http://127.0.0.1:3000/api/health` returns 200, so the task is HEALTHY.
- Example 2: a nurse checking a pulse instead of trusting "I'm fine".
- Why: "the process started" is not "the app works"; only a healthy version gets traffic.

**Blue/green with drain**: the new version (green) runs next to the live one (blue); traffic moves only when green is healthy, and blue stays 5 minutes so open requests finish.
- Example 1: deployment 4 goes live, deployment 3 becomes `superseded` and is deleted at +5 min.
- Example 2: a relay handover: both runners run side by side for a few steps.
- Why: a broken push never takes the working version down.

**Cloud Map**: AWS's private phone book for services inside the VPC.
- Example 1: `d-4.apps.foyr.internal` resolves to the task's private IP.
- Example 2: an office extension list: dial the name, not the desk.
- Why: authproxy (step 8) finds the live task by name, without a load balancer per app.

## Debugging a deploy
```sql
SELECT id, status, failure_step, left(failure_reason, 300), ecs_service, live_at FROM deployments ORDER BY id DESC LIMIT 5;
```
```sql
SELECT id, full_name, live_deployment_id FROM apps;
```
```sql
SELECT kind, deployment_id, status, run_after, last_error FROM jobs ORDER BY id DESC LIMIT 10;
```
Each Start and Health check step links to the task's log stream in `/foyr/apps`.
In the ECS console, the cluster `foyr` lists one service per running deployment.

## Code map
| Path | What it does |
|---|---|
| `internal/run/run.go` | deploy runner (start, watch, live, fail), retirer, the health check command |
| `internal/awsx/runtime.go` | ECS task definitions, services and tasks; Cloud Map names |
| `internal/store/runs.go` | `SetRun`, `MarkLive` (supersede + retire job), `MarkRetired` |
| `internal/db/migrations/003_runs.sql` | new statuses, `apps.live_deployment_id` |
| `scripts/infra-env.sh` | runtime settings from `tofu output` |
