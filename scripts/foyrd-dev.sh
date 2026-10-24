#!/usr/bin/env bash
# Runs foyrd locally with the dev GitHub App and the smee webhook relay (docs/dev/github-app.md).
set -euo pipefail
ENV_FILE="${FOYRD_ENV:-$HOME/.config/foyr/foyrd-dev.env}"
[ -f "$ENV_FILE" ] || { echo "missing $ENV_FILE (see docs/dev/github-app.md)" >&2; exit 1; }
INFRA_ENV="$HOME/.config/foyr/infra.env"
set -a; source "$ENV_FILE"; [ -f "$INFRA_ENV" ] && source "$INFRA_ENV"; set +a
cd "$(dirname "$0")/.."
export AWS_PROFILE="${AWS_PROFILE:-foyr}"
export FOYR_DATABASE_URL="${FOYR_DATABASE_URL:-postgres://$USER@localhost:5432/foyr_dev?sslmode=disable}"
! command -v pg_isready >/dev/null || pg_isready -q -d "$FOYR_DATABASE_URL" || { echo "Postgres is not running (see docs/dev/database.md)" >&2; exit 1; }
ADDR="${FOYR_ADDR:-:8080}"
ROOT="$PWD"; FOYRD_PORT="${ADDR##*:}"; source scripts/login-dev-env.sh
# Magic-link login: the key signs sessions, emails go to this log, apps open at <slug>.apps.localhost:8081.
export FOYR_SESSION_KEY="$(cat "$DEV_KEY")"
PROXY_ADDR="${AUTHPROXY_ADDR:-:8081}"
export FOYR_APPS_DOMAIN="${FOYR_APPS_DOMAIN:-apps.localhost:${PROXY_ADDR##*:}}"
npx --yes smee-client --url "$SMEE_URL" --target "http://localhost:${ADDR##*:}/webhooks/github" &
SMEE=$!
trap 'kill $SMEE 2>/dev/null' EXIT
go run ./cmd/foyrd
