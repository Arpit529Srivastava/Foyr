-- Customer sign-in to the dashboard: who belongs to which GitHub account, and the dashboard's own
-- login tokens and sessions. Kept apart from app login so an app visitor can never become a dashboard user.

CREATE TABLE account_members (
    installation_id bigint NOT NULL REFERENCES installations (id),
    user_id         bigint NOT NULL REFERENCES users (id),
    role            text   NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (installation_id, user_id)
);
CREATE INDEX account_members_user_idx ON account_members (user_id);

-- Only the SHA-256 of a token or session value is stored, never the value.
CREATE TABLE dashboard_login_tokens (
    token_hash bytea PRIMARY KEY,
    email      text NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX dashboard_login_tokens_expiry_idx ON dashboard_login_tokens (expires_at);

CREATE TABLE dashboard_sessions (
    token_hash bytea PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);
CREATE INDEX dashboard_sessions_user_idx ON dashboard_sessions (user_id);
CREATE INDEX dashboard_sessions_expiry_idx ON dashboard_sessions (expires_at);
