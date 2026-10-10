-- Step 7: run built images on ECS and track which deployment is live (ADR-029).
ALTER TABLE deployments DROP CONSTRAINT deployments_status_check;
ALTER TABLE deployments ADD CONSTRAINT deployments_status_check CHECK (status IN
    ('queued', 'building', 'built', 'deploying', 'live', 'superseded', 'failed', 'canceled'));

-- A commit that is running or ran must not build again on a redelivered webhook.
DROP INDEX deployments_one_per_commit;
CREATE UNIQUE INDEX deployments_one_per_commit ON deployments (app_id, commit_sha)
    WHERE status IN ('queued', 'building', 'built', 'deploying', 'live', 'superseded');

ALTER TABLE deployments
    ADD COLUMN task_definition text NOT NULL DEFAULT '',
    ADD COLUMN ecs_service     text NOT NULL DEFAULT '',
    ADD COLUMN live_at         timestamptz,
    ADD COLUMN retired_at      timestamptz;

ALTER TABLE apps ADD COLUMN live_deployment_id bigint REFERENCES deployments (id);
