#!/usr/bin/env bash
# Writes foyrd's AWS runtime settings from `tofu output` to ~/.config/foyr/infra.env (docs/dev/builds.md).
set -euo pipefail
cd "$(dirname "$0")/../infra"
OUT="${1:-$HOME/.config/foyr/infra.env}"
export AWS_PROFILE="${AWS_PROFILE:-foyr}"
json=$(tofu output -json)
get() { printf '%s' "$json" | python3 -c "import json,sys; v=json.load(sys.stdin)$1; print(','.join(v) if isinstance(v,list) else v)"; }
mkdir -p "$(dirname "$OUT")"
cat > "$OUT" <<ENV
FOYR_ECS_CLUSTER=$(get '["ecs_cluster_name"]["value"]')
FOYR_TENANT_SUBNETS=$(get '["tenant_subnet_ids"]["value"]')
FOYR_TENANT_SECURITY_GROUP=$(get '["tenant_security_group_id"]["value"]')
FOYR_TENANT_EXEC_ROLE_ARN=$(get '["exec_role_arns"]["value"]["tenant"]')
FOYR_APPS_NAMESPACE_ID=$(get '["apps_namespace_id"]["value"]')
FOYR_APPS_LOG_GROUP=$(get '["apps_log_group_name"]["value"]')
ENV
echo "wrote $OUT"
