-- Step 8: each app gets a hostname slug, <slug>.<APPS_DOMAIN> (ADR-030).
ALTER TABLE apps ADD COLUMN slug text UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$');

-- Existing apps: repo name made safe; duplicates get their id appended. New apps get one from the store.
WITH base AS (
    SELECT id, left(trim(both '-' from regexp_replace(lower(split_part(full_name, '/', 2)), '[^a-z0-9]+', '-', 'g')), 34) AS s
    FROM apps
), ranked AS (
    SELECT id, s, row_number() OVER (PARTITION BY s ORDER BY id) AS n FROM base
)
UPDATE apps SET slug = CASE
        WHEN length(r.s) < 3 OR r.s IN ('app', 'api', 'auth', 'www', 'docs', 'admin', 'status', 'mail', 'static', 'cdn', 'foyr')
            THEN 'app-' || apps.id
        WHEN r.n > 1 THEN r.s || '-' || apps.id
        ELSE r.s
    END
FROM ranked r WHERE r.id = apps.id;
