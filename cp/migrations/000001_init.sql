-- +goose Up

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE tenants (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE hosts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID NOT NULL REFERENCES tenants(id),
    hostname           TEXT NOT NULL,
    labels             JSONB NOT NULL DEFAULT '{}',
    status             TEXT NOT NULL DEFAULT 'enrolling'
                           CHECK (status IN ('enrolling','active','stale','needs_rebootstrap','revoked')),
    current_generation INT NOT NULL DEFAULT 0,
    last_heartbeat_at  TIMESTAMPTZ,
    agent_version      TEXT,
    kernel_version     TEXT,
    interfaces         JSONB NOT NULL DEFAULT '[]',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX hosts_tenant_status ON hosts (tenant_id, status);
CREATE INDEX hosts_labels_gin    ON hosts USING gin (labels);

CREATE TABLE policies (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    priority        INT  NOT NULL DEFAULT 1000,
    selector        JSONB NOT NULL DEFAULT '{}',
    current_version INT  NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX policies_tenant ON policies (tenant_id);

CREATE TABLE rules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id  UUID NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    priority   INT  NOT NULL DEFAULT 100,
    direction  TEXT NOT NULL CHECK (direction IN ('ingress','egress')),
    match      JSONB NOT NULL DEFAULT '{}',
    state      TEXT CHECK (state IN ('new','established','related','invalid')),
    action     TEXT NOT NULL CHECK (action IN ('allow','deny','log')),
    log        BOOL NOT NULL DEFAULT FALSE,
    silent     BOOL NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX rules_policy_priority ON rules (policy_id, priority);

CREATE TABLE object_groups (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id),
    type           TEXT NOT NULL CHECK (type IN ('ipset','portset','hostset')),
    name           TEXT NOT NULL,
    spec           JSONB NOT NULL DEFAULT '{}',
    resolved_cidrs JSONB,
    resolved_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX object_groups_tenant ON object_groups (tenant_id);

CREATE TABLE rule_objectgroup_refs (
    rule_id         UUID NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    object_group_id UUID NOT NULL REFERENCES object_groups(id) ON DELETE RESTRICT,
    role            TEXT NOT NULL CHECK (role IN ('src','dst')),
    PRIMARY KEY (rule_id, object_group_id, role)
);

CREATE INDEX rule_objectgroup_refs_og ON rule_objectgroup_refs (object_group_id);

CREATE TABLE default_policies (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id              UUID NOT NULL UNIQUE REFERENCES tenants(id),
    default_ingress_action TEXT NOT NULL DEFAULT 'allow' CHECK (default_ingress_action IN ('allow','deny')),
    default_egress_action  TEXT NOT NULL DEFAULT 'allow' CHECK (default_egress_action IN ('allow','deny')),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down

DROP TABLE IF EXISTS default_policies;
DROP TABLE IF EXISTS rule_objectgroup_refs;
DROP TABLE IF EXISTS object_groups;
DROP TABLE IF EXISTS rules;
DROP TABLE IF EXISTS policies;
DROP TABLE IF EXISTS hosts;
DROP TABLE IF EXISTS tenants;
DROP EXTENSION IF EXISTS "pgcrypto";
