-- +goose Up

CREATE TABLE certificates (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID NOT NULL REFERENCES tenants(id),
    host_id    UUID NOT NULL REFERENCES hosts(id),
    serial     TEXT NOT NULL UNIQUE,
    not_before TIMESTAMPTZ NOT NULL,
    not_after  TIMESTAMPTZ NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active'
                   CHECK (status IN ('active','rotated','revoked','expired')),
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX certificates_host   ON certificates (host_id);
CREATE INDEX certificates_status ON certificates (status);

CREATE TABLE bootstrap_tokens (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL REFERENCES tenants(id),
    type             TEXT NOT NULL CHECK (type IN ('single_host','bulk')),
    token_hash       TEXT NOT NULL UNIQUE,
    target_host_id   UUID REFERENCES hosts(id),
    label_template   JSONB,
    max_uses         INT NOT NULL DEFAULT 1,
    uses_count       INT NOT NULL DEFAULT 0,
    expires_at       TIMESTAMPTZ NOT NULL,
    created_by       UUID,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX bootstrap_tokens_tenant ON bootstrap_tokens (tenant_id);

-- +goose Down

DROP TABLE IF EXISTS bootstrap_tokens;
DROP TABLE IF EXISTS certificates;
