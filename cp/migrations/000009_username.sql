-- +goose Up

ALTER TABLE users ADD COLUMN username TEXT;

-- Populate existing rows: use email local part, append _N on collision
DO $$
DECLARE
    r RECORD;
    base TEXT;
    candidate TEXT;
    n INT;
BEGIN
    FOR r IN SELECT id, email FROM users ORDER BY created_at LOOP
        base := split_part(r.email, '@', 1);
        -- Sanitize: keep only [a-zA-Z0-9-], replace anything else with '-'
        base := regexp_replace(base, '[^a-zA-Z0-9-]', '-', 'g');
        -- Strip leading/trailing dashes
        base := regexp_replace(base, '^-+|-+$', '', 'g');
        -- Ensure non-empty
        IF base = '' THEN base := 'user'; END IF;
        candidate := base;
        n := 1;
        WHILE EXISTS (SELECT 1 FROM users WHERE username = candidate AND id != r.id) LOOP
            candidate := base || '-' || n;
            n := n + 1;
        END LOOP;
        UPDATE users SET username = candidate WHERE id = r.id;
    END LOOP;
END$$;

ALTER TABLE users ADD CONSTRAINT users_tenant_username_unique UNIQUE (tenant_id, username);
ALTER TABLE users ALTER COLUMN username SET NOT NULL;

-- +goose Down

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_tenant_username_unique;
ALTER TABLE users DROP COLUMN IF EXISTS username;
