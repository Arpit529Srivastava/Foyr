#!/usr/bin/env bash
# Copies foyrd's secrets from ~/.config/foyr into SSM /foyr/platform/* (ADR-032). Never prints them.
set -euo pipefail
ENV_FILE="${FOYRD_ENV:-$HOME/.config/foyr/foyrd-dev.env}"
[ -f "$ENV_FILE" ] || { echo "missing $ENV_FILE (see docs/dev/github-app.md)" >&2; exit 1; }
set -a; source "$ENV_FILE"; set +a
export AWS_PROFILE="${AWS_PROFILE:-foyr}" AWS_REGION="${AWS_REGION:-ap-south-1}"
: "${FOYR_GITHUB_WEBHOOK_SECRET:?not set in $ENV_FILE}" "${FOYR_INTERNAL_TOKEN:?not set in $ENV_FILE}" "${FOYR_GITHUB_PRIVATE_KEY_PATH:?not set in $ENV_FILE}"
umask 077
# The founder page key (ADR-033) is made once and kept next to the other secrets.
FOUNDER_KEY_FILE="$HOME/.config/foyr/founder-key"
[ -s "$FOUNDER_KEY_FILE" ] || { openssl rand -hex 32 | tr -d '\n' > "$FOUNDER_KEY_FILE"; echo "made $FOUNDER_KEY_FILE"; }
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
printf '%s' "$FOYR_GITHUB_WEBHOOK_SECRET" > "$tmp/webhook"
printf '%s' "$FOYR_INTERNAL_TOKEN" > "$tmp/internal"
put() { aws ssm put-parameter --name "/foyr/platform/$1" --type SecureString --overwrite --value "file://$2" --query Version --output text | sed "s|^|/foyr/platform/$1 version |"; }
put github-webhook-secret "$tmp/webhook"
put internal-token "$tmp/internal"
put github-private-key "$FOYR_GITHUB_PRIVATE_KEY_PATH"
put founder-key "$FOUNDER_KEY_FILE"
