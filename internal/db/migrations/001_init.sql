-- Step 6a: installations, apps, deployments, the job queue and the audit trail (ADR-025, ADR-026).
-- Every tenant-owned row carries installation_id, the tenant key.

CREATE TABLE installations (
    id            bigint PRIMARY KEY, -- GitHub installation id
    account_login text NOT NULL,
    suspended_at  timestamptz,
    removed_at    timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE apps (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    installation_id bigint NOT NULL REFERENCES installations (id),
    github_repo_id  bigint NOT NULL UNIQUE,
    full_name       text   NOT NULL,
    default_branch  text   NOT NULL DEFAULT 'main',
    private         boolean NOT NULL DEFAULT true,
    removed_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX apps_installation_idx ON apps (installation_id);

CREATE TABLE deployments (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    app_id          bigint NOT NULL REFERENCES apps (id),
    installation_id bigint NOT NULL REFERENCES installations (id),
    commit_sha      text   NOT NULL CHECK (commit_sha ~ '^[0-9a-f]{40}$'),
    branch          text   NOT NULL,
    commit_message  text   NOT NULL DEFAULT '',
    commit_url      text   NOT NULL DEFAULT '',
    pusher_login    text   NOT NULL DEFAULT '',
    pusher_id       bigint,
    trigger         text   NOT NULL DEFAULT 'push' CHECK (trigger IN ('push', 'manual', 'rollback')),
    delivery_id     text   NOT NULL DEFAULT '',
    trace_id        text   NOT NULL DEFAULT '',
    status          text   NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued', 'building', 'built', 'failed', 'canceled')),
    failure_step    text,
    failure_reason  text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    started_at      timestamptz,
    finished_at     timestamptz
);
-- One live deployment per commit, so a redelivered webhook cannot build twice.
CREATE UNIQUE INDEX deployments_one_per_commit ON deployments (app_id, commit_sha)
    WHERE status IN ('queued', 'building', 'built');
CREATE INDEX deployments_app_recent_idx ON deployments (app_id, created_at DESC, id DESC);
CREATE INDEX deployments_installation_recent_idx ON deployments (installation_id, created_at DESC, id DESC);

CREATE TABLE deployment_steps (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    deployment_id bigint NOT NULL REFERENCES deployments (id) ON DELETE CASCADE,
    name          text   NOT NULL,
    position      int    NOT NULL,
    status        text   NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'skipped')),
    log_ref       text   NOT NULL DEFAULT '',
    started_at    timestamptz,
    finished_at   timestamptz,
    UNIQUE (deployment_id, name)
);

-- The developer-facing timeline of a deployment.
CREATE TABLE deployment_events (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    deployment_id bigint NOT NULL REFERENCES deployments (id) ON DELETE CASCADE,
    occurred_at   timestamptz NOT NULL DEFAULT now(),
    from_status   text,
    to_status     text   NOT NULL,
    message       text   NOT NULL DEFAULT ''
);
CREATE INDEX deployment_events_deployment_idx ON deployment_events (deployment_id, id);

CREATE TABLE jobs (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind            text   NOT NULL,
    deployment_id   bigint REFERENCES deployments (id),
    installation_id bigint NOT NULL,
    trace_id        text   NOT NULL DEFAULT '',
    status          text   NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    attempts        int    NOT NULL DEFAULT 0,
    max_attempts    int    NOT NULL DEFAULT 3,
    run_after       timestamptz NOT NULL DEFAULT now(),
    locked_by       text,
    lease_until     timestamptz,
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    started_at      timestamptz,
    finished_at     timestamptz
);
CREATE INDEX jobs_ready_idx ON jobs (run_after, id) WHERE status = 'queued';
CREATE INDEX jobs_running_idx ON jobs (installation_id, lease_until) WHERE status = 'running';
CREATE INDEX jobs_finished_idx ON jobs (finished_at) WHERE status IN ('done', 'failed');

-- Every webhook GitHub sent us, kept 30 days.
CREATE TABLE webhook_deliveries (
    delivery_id     text PRIMARY KEY,
    event           text NOT NULL,
    action          text NOT NULL DEFAULT '',
    installation_id bigint,
    repo_full_name  text NOT NULL DEFAULT '',
    result          text NOT NULL CHECK (result IN ('ok', 'ignored', 'error')),
    detail          text NOT NULL DEFAULT '',
    duration_ms     int  NOT NULL DEFAULT 0,
    attempts        int  NOT NULL DEFAULT 1,
    trace_id        text NOT NULL DEFAULT '',
    received_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX webhook_deliveries_received_idx ON webhook_deliveries (received_at);
CREATE INDEX webhook_deliveries_installation_idx ON webhook_deliveries (installation_id, received_at DESC);

-- Who did what, when. Append-only and split into one partition per month.
CREATE TABLE audit_events (
    id              bigint GENERATED ALWAYS AS IDENTITY,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    installation_id bigint,
    app_id          bigint,
    actor_type      text NOT NULL CHECK (actor_type IN ('github_user', 'foyr_user', 'system')),
    actor_id        text NOT NULL DEFAULT '',
    actor_login     text NOT NULL DEFAULT '',
    action          text NOT NULL,
    target_type     text NOT NULL,
    target_id       text NOT NULL,
    trace_id        text NOT NULL DEFAULT '',
    details         jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY (occurred_at, id)
) PARTITION BY RANGE (occurred_at);
-- Catches rows outside the monthly partitions so inserts never fail.
CREATE TABLE audit_events_default PARTITION OF audit_events DEFAULT;
CREATE INDEX audit_events_installation_idx ON audit_events (installation_id, occurred_at DESC);
CREATE INDEX audit_events_target_idx ON audit_events (target_type, target_id, occurred_at DESC);

CREATE FUNCTION audit_events_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END
$$;
CREATE TRIGGER audit_events_no_update_delete BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_append_only();
CREATE TRIGGER audit_events_no_truncate BEFORE TRUNCATE ON audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION audit_events_append_only();
