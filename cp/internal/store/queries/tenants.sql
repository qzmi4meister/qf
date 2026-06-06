-- name: GetTenant :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: CreateTenant :one
INSERT INTO tenants (slug, name) VALUES ($1, $2) RETURNING *;

-- name: GetDefaultPolicy :one
SELECT * FROM default_policies WHERE tenant_id = $1;

-- name: UpsertDefaultPolicy :one
INSERT INTO default_policies (tenant_id, default_ingress_action, default_egress_action)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id) DO UPDATE
SET default_ingress_action = EXCLUDED.default_ingress_action,
    default_egress_action  = EXCLUDED.default_egress_action,
    updated_at             = NOW()
RETURNING *;
