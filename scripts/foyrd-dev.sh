#!/usr/bin/env bash
# Runs foyrd locally with the dev GitHub App and the smee webhook relay (docs/dev/github-app.md).
set -euo pipefail
ENV_FILE="${FOYRD_ENV:-$HOME/.config/foyr/foyrd-dev.env}"
[ -f "$ENV_FILE" ] || { echo "missing $ENV_FILE (see docs/dev/github-app.md)" >&2; exit 1; }
set -a; source "$ENV_FILE"; set +a
cd "$(dirname "$0")/.."
export FOYR_DATABASE_URL="${FOYR_DATABASE_URL:-postgres://$USER@localhost:5432/foyr_dev?sslmode=disable}"
! command -v pg_isready >/dev/null || pg_isready -q -d "$FOYR_DATABASE_URL" || { echo "Postgres is not running (see docs/dev/database.md)" >&2; exit 1; }
ADDR="${FOYR_ADDR:-:8080}"
npx --yes smee-client --url "$SMEE_URL" --target "http://localhost:${ADDR##*:}/webhooks/github" &
SMEE=$!
trap 'kill $SMEE 2>/dev/null' EXIT
go run ./cmd/foyrd
