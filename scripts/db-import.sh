#!/usr/bin/env bash
# One-time copy of the local foyr_dev database into RDS, before foyrd first starts in AWS (ADR-032).
# The database is private, so a one-off Fargate task with psql does the restore inside the VPC.
set -euo pipefail
export AWS_PROFILE="${AWS_PROFILE:-foyr}" AWS_REGION="${AWS_REGION:-ap-south-1}"
export PATH="/opt/homebrew/opt/postgresql@17/bin:$PATH"
SRC="${1:-foyr_dev}"
cd "$(dirname "$0")/../infra"

running=$(aws ecs describe-services --cluster foyr --services foyrd --query 'services[0].runningCount' --output text)
[ "$running" = "0" ] || { echo "foyrd is running in AWS ($running tasks); the import is only for an empty database" >&2; exit 1; }

# Whole database, schema included, so foyrd sees its migrations as applied. Staged in S3 for 15 minutes.
account=$(aws sts get-caller-identity --query Account --output text)
key="s3://foyr-tfstate-$account/tmp/dbimport-$(date +%s).sql.gz"
pg_dump --no-owner --no-privileges -d "$SRC" | gzip -9 | aws s3 cp - "$key" --only-show-errors
trap 'aws s3 rm "$key" --only-show-errors' EXIT
url=$(aws s3 presign "$key" --expires-in 900)
echo "dump of $SRC staged"

json=$(tofu output -json)
out() { printf '%s' "$json" | python3 -c "import json,sys; v=json.load(sys.stdin)$1; print(','.join(v) if isinstance(v,list) else v)"; }
subnets=$(out '["platform_subnet_ids"]["value"]')
sg=$(aws ec2 describe-security-groups --filters Name=group-name,Values=foyr-foyrd --query 'SecurityGroups[0].GroupId' --output text)
exec_role=$(out '["exec_role_arns"]["value"]["platform"]')

td=$(aws ecs register-task-definition --family foyr-dbimport --requires-compatibilities FARGATE --network-mode awsvpc \
  --cpu 256 --memory 512 --execution-role-arn "$exec_role" \
  --runtime-platform cpuArchitecture=ARM64,operatingSystemFamily=LINUX \
  --container-definitions "$(cat <<JSON
[{"name":"psql","image":"public.ecr.aws/docker/library/postgres:17-alpine","essential":true,
  "command":["sh","-c","set -eo pipefail; wget -qO- \"\$DUMP_URL\" | gunzip | psql \"\$FOYR_DATABASE_URL\" -q -v ON_ERROR_STOP=1 --single-transaction && echo deployments: && psql \"\$FOYR_DATABASE_URL\" -Atc 'select count(*) from deployments'"],
  "secrets":[{"name":"FOYR_DATABASE_URL","valueFrom":"arn:aws:ssm:$AWS_REGION:$account:parameter/foyr/platform/database-url"}],
  "logConfiguration":{"logDriver":"awslogs","options":{"awslogs-group":"/foyr/platform","awslogs-region":"$AWS_REGION","awslogs-stream-prefix":"dbimport"}}}]
JSON
)" --query taskDefinition.taskDefinitionArn --output text)
trap 'aws ecs deregister-task-definition --task-definition "$td" >/dev/null; aws s3 rm "$key" --only-show-errors' EXIT

task=$(aws ecs run-task --cluster foyr --task-definition "$td" --launch-type FARGATE \
  --network-configuration "awsvpcConfiguration={subnets=[$subnets],securityGroups=[$sg],assignPublicIp=DISABLED}" \
  --overrides "{\"containerOverrides\":[{\"name\":\"psql\",\"environment\":[{\"name\":\"DUMP_URL\",\"value\":\"$url\"}]}]}" \
  --query 'tasks[0].taskArn' --output text)
echo "import task ${task##*/} started; waiting..."
aws ecs wait tasks-stopped --cluster foyr --tasks "$task"
code=$(aws ecs describe-tasks --cluster foyr --tasks "$task" --query 'tasks[0].containers[0].exitCode' --output text)
aws logs get-log-events --log-group-name /foyr/platform --log-stream-name "dbimport/psql/${task##*/}" \
  --query 'events[].message' --output text 2>/dev/null | tr '\t' '\n' | tail -20 || true
[ "$code" = "0" ] && echo "import done" || { echo "import failed (exit $code); the database is unchanged (single transaction)" >&2; exit 1; }
