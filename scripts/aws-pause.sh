#!/usr/bin/env bash
# Pauses Foyr in AWS to save money, or resumes it: scripts/aws-pause.sh pause|resume.
# Paused: apps, foyrd and authproxy run 0 tasks and the database is stopped (AWS restarts it after 7 days).
# The load balancer, NAT instance and DNS keep running; pushes while paused are not deployed.
set -euo pipefail
export AWS_PROFILE="${AWS_PROFILE:-foyr}" AWS_REGION="${AWS_REGION:-ap-south-1}"
CLUSTER=foyr DB=foyr
services() { aws ecs list-services --cluster "$CLUSTER" --query 'serviceArns' --output text | tr '\t' '\n' | sed 's#.*/##' | grep -v '^$'; }
scale() { aws ecs update-service --cluster "$CLUSTER" --service "$1" --desired-count "$2" --query 'service.serviceName' --output text >/dev/null; echo "$1 -> $2 tasks"; }
db_status() { aws rds describe-db-instances --db-instance-identifier "$DB" --query 'DBInstances[0].DBInstanceStatus' --output text; }

case "${1:-}" in
pause)
  # Apps and authproxy first, then foyrd, so nothing new starts while the database stops.
  for s in $(services | grep '^d-'); do scale "$s" 0; done
  scale authproxy 0
  scale foyrd 0
  aws ecs wait services-stable --cluster "$CLUSTER" --services foyrd authproxy
  [ "$(db_status)" = available ] && aws rds stop-db-instance --db-instance-identifier "$DB" --query 'DBInstance.DBInstanceStatus' --output text | sed 's/^/database: /'
  echo "paused"
  ;;
resume)
  if [ "$(db_status)" != available ]; then
    [ "$(db_status)" = stopped ] && aws rds start-db-instance --db-instance-identifier "$DB" >/dev/null
    echo "starting the database (about 5 minutes)"
    aws rds wait db-instance-available --db-instance-identifier "$DB"
  fi
  scale foyrd 1
  scale authproxy 1
  for s in $(services | grep '^d-'); do scale "$s" 1; done
  aws ecs wait services-stable --cluster "$CLUSTER" --services foyrd authproxy
  echo "resumed; apps are healthy again within a minute or two"
  ;;
*) echo "usage: $0 pause|resume" >&2; exit 2 ;;
esac
