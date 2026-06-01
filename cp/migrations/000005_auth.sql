-- +goose Up

-- ── Users ─────────────────────────────────────────────────────────────────
CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL REFERENCES tenants(id),
    email         TEXT        NOT NULL,
    password_hash TEXT,                              -- NULL for OIDC-only accounts
    oidc_subject  TEXT,
    status        TEXT        NOT NULL DEFAULT 'active'
                              CHECK (status IN ('active', 'disabled')),
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, email)
);

CREATE INDEX users_tenant ON users (tenant_id);

-- ── User roles ────────────────────────────────────────────────────────────
CREATE TABLE user_roles (
    user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    role      TEXT NOT NULL CHECK (role IN ('admin', 'editor', 'operator', 'auditor')),
    PRIMARY KEY (user_id, tenant_id)
);

CREATE INDEX user_roles_tenant ON user_roles (tenant_id, role);

-- ── API tokens ────────────────────────────────────────────────────────────
CREATE TABLE api_tokens (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID        NOT NULL REFERENCES tenants(id),
    name         TEXT        NOT NULL,
    token_hash   TEXT        NOT NULL UNIQUE,
    role         TEXT        NOT NULL CHECK (role IN ('admin', 'editor', 'operator', 'auditor')),
    created_by   UUID        REFERENCES users(id) ON DELETE SET NULL,
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX api_tokens_tenant ON api_tokens (tenant_id);

-- +goose Down

DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS users;
