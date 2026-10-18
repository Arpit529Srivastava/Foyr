#!/usr/bin/env bash
# Copies the founder page key to the clipboard and opens the sign-in page (ADR-033). Never prints the key.
set -euo pipefail
FILE="$HOME/.config/foyr/founder-key"
[ -s "$FILE" ] || { echo "no $FILE yet; run scripts/platform-secrets.sh" >&2; exit 1; }
pbcopy < "$FILE"
open "${1:-https://api.foyr.online/founder}"
echo "key copied; paste it into the sign-in box, then clear the clipboard"
