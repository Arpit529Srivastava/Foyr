# Sourced by foyrd-dev.sh and authproxy-dev.sh: throwaway login keys and URLs for local development (docs/dev/login.md).
# Needs ROOT (the repo root); FOYRD_PORT picks foyrd's port. Keys live in ~/.config/foyr/dev-session.{key,pub}, made on first use.
DEV_KEY="$HOME/.config/foyr/dev-session.key"
DEV_PUB="$HOME/.config/foyr/dev-session.pub"
if [ ! -f "$DEV_KEY" ] || [ ! -f "$DEV_PUB" ]; then
  mkdir -p "$HOME/.config/foyr"
  _pair="$(cd "$ROOT" && go run ./cmd/sessionkey)"
  (umask 077; printf '%s\n' "$_pair" | sed -n 's/^seed=//p' > "$DEV_KEY")
  printf '%s\n' "$_pair" | sed -n 's/^public=//p' > "$DEV_PUB"
  unset _pair
fi
# foyrd signs with the key, authproxy only verifies with the public half.
# Sign-in pages are served by foyrd, so the auth URL always uses foyrd's port (FOYRD_PORT, default 8080).
export FOYR_AUTH_URL="${FOYR_AUTH_URL:-http://localhost:${FOYRD_PORT:-8080}}"
