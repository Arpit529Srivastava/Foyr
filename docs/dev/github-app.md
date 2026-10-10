# Running foyrd with the dev GitHub App

foyrd receives GitHub webhooks on `POST /webhooks/github`. Locally, GitHub cannot reach your laptop,
so a smee.io channel relays webhooks to `localhost` (ADR-023).

## One-time setup
1. Create a channel at https://smee.io (click "Start a new channel") and copy its URL.
2. Register the `foyr-dev` GitHub App (Settings → Developer settings → GitHub Apps → New):
   webhook URL = the smee URL, a random webhook secret (`openssl rand -hex 32`),
   permissions Contents: Read-only and Metadata: Read-only, event: Push, "Only on this account".
3. Generate a private key and move it out of Downloads:
   ```bash
   mkdir -p ~/.config/foyr && mv ~/Downloads/foyr-dev.*.private-key.pem ~/.config/foyr/github-app-dev.pem && chmod 600 ~/.config/foyr/github-app-dev.pem
   ```
4. Install the App on the test repo only.
5. Create `~/.config/foyr/foyrd-dev.env` (never commit it):
   ```bash
   FOYR_GITHUB_APP_ID=<App ID from the App's settings page>
   FOYR_GITHUB_WEBHOOK_SECRET=<the secret from step 2>
   FOYR_GITHUB_PRIVATE_KEY_PATH=$HOME/.config/foyr/github-app-dev.pem
   SMEE_URL=<the smee URL>
   ```

## Every time
Postgres must be running (docs/dev/database.md), then:
```bash
scripts/foyrd-dev.sh
```
It starts foyrd on `:8080` and the smee relay, applies database migrations and logs `database ready`.
Push to the test repo's default branch and watch for `deployment queued`, then `build started`,
`build succeeded` (or `build failed`) in the JSON logs. Workers need AWS: run `aws sso login --profile foyr`
first (the script uses `AWS_PROFILE=foyr`), or set `FOYR_WORKERS=0` to only record pushes.
Queries in docs/dev/database.md; how builds work in docs/dev/builds.md.

## Settings foyrd reads
| Variable | Required | Notes |
|---|---|---|
| `FOYR_GITHUB_APP_ID` | yes | positive number |
| `FOYR_GITHUB_WEBHOOK_SECRET` | yes | at least 16 characters |
| `FOYR_GITHUB_PRIVATE_KEY_PATH` or `FOYR_GITHUB_PRIVATE_KEY` | yes | file path locally, PEM text in AWS |
| `FOYR_DATABASE_URL` | yes | the dev script defaults to local `foyr_dev` |
| `FOYR_WORKERS` | no | build workers, default 2; `0` turns building off |
| `FOYR_AWS_REGION` | no | default `ap-south-1` |
| `FOYR_CODEBUILD_PROJECT` | no | default `foyr-build` |
| `FOYR_TENANT_SUBNETS`, `FOYR_TENANT_SECURITY_GROUP`, `FOYR_TENANT_EXEC_ROLE_ARN`, `FOYR_APPS_NAMESPACE_ID` | with workers | from `scripts/infra-env.sh` (docs/dev/deploys.md) |
| `FOYR_ECS_CLUSTER`, `FOYR_APPS_LOG_GROUP`, `FOYR_APPS_CAPACITY` | no | `foyr`, `/foyr/apps`, `FARGATE_SPOT` |
| `FOYR_ADDR` | no | default `:8080` |
| `FOYR_GITHUB_API_URL` | no | default `https://api.github.com` |
