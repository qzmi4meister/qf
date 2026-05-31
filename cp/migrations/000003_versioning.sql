-- +goose Up

CREATE TABLE config_versions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    entity_type TEXT NOT NULL CHECK (entity_type IN ('policy','objectgroup','defaultpolicy')),
    entity_id   UUID NOT NULL,
    version     INT  NOT NULL,
    content     JSONB NOT NULL,
    created_by  TEXT NOT NULL DEFAULT 'system',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX config_versions_entity ON config_versions (tenant_id, entity_type, entity_id, version);

CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    actor_type  TEXT NOT NULL CHECK (actor_type IN ('user','api_token','system')),
    actor_id    UUID,
    action      TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id   UUID,
    before      JSONB,
    after       JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_log_tenant_time  ON audit_log (tenant_id, created_at);
CREATE INDEX audit_log_object       ON audit_log (object_type, object_id);

-- +goose Down

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS config_versions;
