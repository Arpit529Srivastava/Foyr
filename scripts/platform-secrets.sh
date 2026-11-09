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
# The session signing key (ADR-035) is made once. macOS's openssl cannot make Ed25519 keys, so a Go command does.
SESSION_KEY_FILE="$HOME/.config/foyr/session.key"; SESSION_PUB_FILE="$HOME/.config/foyr/session.pub"
if [ ! -s "$SESSION_KEY_FILE" ] || [ ! -s "$SESSION_PUB_FILE" ]; then
  pair="$(cd "$(dirname "$0")/.." && go run ./cmd/sessionkey)"
  printf '%s' "$pair" | sed -n 's/^seed=//p' | tr -d '\n' > "$SESSION_KEY_FILE"
  printf '%s' "$pair" | sed -n 's/^public=//p' | tr -d '\n' > "$SESSION_PUB_FILE"
  unset pair; echo "made $SESSION_KEY_FILE and $SESSION_PUB_FILE"
fi
# The assertion pair (authproxy signs, foyrd verifies) is made once, like the session key.
ASSERT_KEY_FILE="$HOME/.config/foyr/assertion.key"; ASSERT_PUB_FILE="$HOME/.config/foyr/assertion.pub"
if [ ! -s "$ASSERT_KEY_FILE" ] && [ ! -s "$ASSERT_PUB_FILE" ]; then
  pair="$(cd "$(dirname "$0")/.." && go run ./cmd/assertionkey)"
  printf '%s' "$pair" | sed -n 's/^key=//p' | tr -d '\n' > "$ASSERT_KEY_FILE"
  printf '%s' "$pair" | sed -n 's/^public=//p' | tr -d '\n' > "$ASSERT_PUB_FILE"
  unset pair; echo "made $ASSERT_KEY_FILE and $ASSERT_PUB_FILE"
elif [ ! -s "$ASSERT_KEY_FILE" ]; then
  echo "missing $ASSERT_KEY_FILE (its pair $ASSERT_PUB_FILE exists; not regenerating)" >&2; exit 1
elif [ ! -s "$ASSERT_PUB_FILE" ]; then
  echo "missing $ASSERT_PUB_FILE (its pair $ASSERT_KEY_FILE exists; not regenerating)" >&2; exit 1
fi
# Resend's API key: create one at resend.com/api-keys (sending access) and save it in this file.
RESEND_KEY_FILE="$HOME/.config/foyr/resend-api-key"
[ -s "$RESEND_KEY_FILE" ] || { echo "missing $RESEND_KEY_FILE (a Resend API key with sending access; docs/dev/platform.md)" >&2; exit 1; }
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
printf '%s' "$FOYR_GITHUB_WEBHOOK_SECRET" > "$tmp/webhook"
printf '%s' "$FOYR_INTERNAL_TOKEN" > "$tmp/internal"
put() { aws ssm put-parameter --name "/foyr/platform/$1" --type SecureString --overwrite --value "file://$2" --query Version --output text | sed "s|^|/foyr/platform/$1 version |"; }
put github-webhook-secret "$tmp/webhook"
put internal-token "$tmp/internal"
put github-private-key "$FOYR_GITHUB_PRIVATE_KEY_PATH"
put founder-key "$FOUNDER_KEY_FILE"
put session-key "$SESSION_KEY_FILE"
put session-public-key "$SESSION_PUB_FILE"
put resend-api-key "$RESEND_KEY_FILE"
# No --overwrite: a re-run must never replace rotated keys, and only ParameterAlreadyExists is benign.
put_once() {
  local out rc
  out="$(aws ssm put-parameter --name "/foyr/platform/$1" --type SecureString --value "file://$2" --query Version --output text 2>&1)" && rc=0 || rc=$?
  if [ "$rc" -eq 0 ]; then
    echo "/foyr/platform/$1 version $out"
  elif [[ "$out" == *ParameterAlreadyExists* ]]; then
    echo "/foyr/platform/$1 already exists, left alone"
  else
    echo "$out" >&2; exit "$rc"
  fi
}
put_once assertion-key "$ASSERT_KEY_FILE"
put_once assertion-public-keys "$ASSERT_PUB_FILE"
