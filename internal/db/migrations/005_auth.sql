-- Magic-link login: users, per-app members and invites, single-use tokens, sessions (ADR-035).
-- users is global (one person, many apps); every other row carries installation_id (ADR-026).

CREATE TABLE users (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         text NOT NULL UNIQUE CHECK (email = lower(email)),
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz
);

-- Apps that exist today keep serving to everyone; new apps start private.
ALTER TABLE apps ADD COLUMN access_mode text NOT NULL DEFAULT 'private'
    CHECK (access_mode IN ('private', 'public'));
UPDATE apps SET access_mode = 'public';

CREATE TABLE app_members (
    app_id          bigint NOT NULL REFERENCES apps (id),
    user_id         bigint NOT NULL REFERENCES users (id),
    installation_id bigint NOT NULL REFERENCES installations (id),
    role            text   NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, user_id)
);
CREATE INDEX app_members_user_idx ON app_members (user_id);

CREATE TABLE invites (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    app_id          bigint NOT NULL REFERENCES apps (id),
    installation_id bigint NOT NULL REFERENCES installations (id),
    email           text   NOT NULL CHECK (email = lower(email)),
    role            text   NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    invited_by      text   NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    accepted_at     timestamptz,
    revoked_at      timestamptz
);
-- One open invite per app and email.
CREATE UNIQUE INDEX invites_one_open ON invites (app_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

-- Only the SHA-256 of a token or code is stored, never the value.
CREATE TABLE login_tokens (
    token_hash      bytea PRIMARY KEY,
    email           text   NOT NULL,
    app_id          bigint NOT NULL REFERENCES apps (id),
    installation_id bigint NOT NULL REFERENCES installations (id),
    expires_at      timestamptz NOT NULL,
    used_at         timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX login_tokens_expiry_idx ON login_tokens (expires_at);

CREATE TABLE auth_codes (
    code_hash       bytea PRIMARY KEY,
    user_id         bigint NOT NULL REFERENCES users (id),
    app_id          bigint NOT NULL REFERENCES apps (id),
    installation_id bigint NOT NULL REFERENCES installations (id),
    expires_at      timestamptz NOT NULL,
    used_at         timestamptz
);
CREATE INDEX auth_codes_expiry_idx ON auth_codes (expires_at);

CREATE TABLE sessions (
    id              text PRIMARY KEY,
    user_id         bigint NOT NULL REFERENCES users (id),
    app_id          bigint NOT NULL REFERENCES apps (id),
    installation_id bigint NOT NULL REFERENCES installations (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    revoked_at      timestamptz
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_revoked_idx ON sessions (expires_at) WHERE revoked_at IS NOT NULL;
