-- Additive: a new column with a default, so a rolled-back foyrd that inserts only email still works.
-- The id is permanent: usr_ plus 16 lowercase hex characters.
ALTER TABLE users ADD COLUMN public_id text NOT NULL
    DEFAULT 'usr_' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 16);
ALTER TABLE users ADD CONSTRAINT users_public_id_key UNIQUE (public_id);
ALTER TABLE users ADD CONSTRAINT users_public_id_format CHECK (public_id ~ '^usr_[0-9a-f]{16}$');
