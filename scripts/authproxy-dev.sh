#!/usr/bin/env bash
# Runs authproxy locally against foyrd's routes (docs/dev/routing.md). Apps run in AWS, so locally
# every route answers 502 unless FOYR_STATIC_ROUTES points a slug at a local app.
set -euo pipefail
ENV_FILE="${FOYRD_ENV:-$HOME/.config/foyr/foyrd-dev.env}"
[ -f "$ENV_FILE" ] || { echo "missing $ENV_FILE (see docs/dev/github-app.md)" >&2; exit 1; }
set -a; source "$ENV_FILE"; set +a
cd "$(dirname "$0")/.."
export FOYR_ADDR="${AUTHPROXY_ADDR:-:8081}"
export FOYR_APPS_DOMAIN="${FOYR_APPS_DOMAIN:-apps.localhost}"
export FOYR_ROUTES_URL="${FOYR_ROUTES_URL:-http://localhost${FOYR_INTERNAL_ADDR:-:9090}/internal/routes}"
# Magic-link login, against local foyrd (scripts/foyrd-dev.sh makes the keys).
ROOT="$PWD"; source scripts/login-dev-env.sh
export FOYR_SESSION_PUBLIC_KEY="$(cat "$DEV_PUB")"
export FOYR_FOYRD_URL="${FOYR_FOYRD_URL:-http://localhost${FOYR_INTERNAL_ADDR:-:9090}}"
exec go run ./cmd/authproxy
