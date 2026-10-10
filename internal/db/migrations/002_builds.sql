-- Step 6b: link deployments to their CodeBuild run, image and log (ADR-027).
ALTER TABLE deployments
    ADD COLUMN build_id  text NOT NULL DEFAULT '',
    ADD COLUMN image_uri text NOT NULL DEFAULT '',
    ADD COLUMN log_url   text NOT NULL DEFAULT '';

ALTER TABLE deployment_steps
    ADD COLUMN message text NOT NULL DEFAULT '';
