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
Push to the test repo's default branch and watch for `deployment queued` (with `deploy_id`) in the JSON
logs. The deployment, its job, its audit event and the webhook delivery are now in `foyr_dev`
(queries in docs/dev/database.md). Builds start in step 6b.

## Settings foyrd reads
| Variable | Required | Notes |
|---|---|---|
| `FOYR_GITHUB_APP_ID` | yes | positive number |
| `FOYR_GITHUB_WEBHOOK_SECRET` | yes | at least 16 characters |
| `FOYR_GITHUB_PRIVATE_KEY_PATH` or `FOYR_GITHUB_PRIVATE_KEY` | yes | file path locally, PEM text in AWS |
| `FOYR_DATABASE_URL` | yes | the dev script defaults to local `foyr_dev` |
| `FOYR_ADDR` | no | default `:8080` |
| `FOYR_GITHUB_API_URL` | no | default `https://api.github.com` |
