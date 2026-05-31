-- name: GetPolicy :one
SELECT * FROM policies WHERE id = $1 AND tenant_id = $2;

-- name: ListPolicies :many
SELECT * FROM policies WHERE tenant_id = $1 ORDER BY priority, created_at;

-- name: CreatePolicy :one
INSERT INTO policies (tenant_id, name, description, priority, selector)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdatePolicy :one
UPDATE policies
SET name = $3, description = $4, priority = $5, selector = $6,
    current_version = current_version + 1, updated_at = NOW()
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: DeletePolicy :exec
DELETE FROM policies WHERE id = $1 AND tenant_id = $2;

-- name: GetRule :one
SELECT * FROM rules WHERE id = $1;

-- name: ListRulesByPolicy :many
SELECT * FROM rules WHERE policy_id = $1 ORDER BY priority;

-- name: CreateRule :one
INSERT INTO rules (policy_id, name, priority, direction, match, state, action, log, silent)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateRule :one
UPDATE rules
SET name = $2, priority = $3, direction = $4, match = $5,
    state = $6, action = $7, log = $8, silent = $9, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteRule :exec
DELETE FROM rules WHERE id = $1;

-- name: DeleteRulesByPolicy :exec
DELETE FROM rules WHERE policy_id = $1;

-- name: UpsertRuleObjectGroupRef :exec
INSERT INTO rule_objectgroup_refs (rule_id, object_group_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (rule_id, object_group_id, role) DO NOTHING;

-- name: DeleteRuleObjectGroupRefs :exec
DELETE FROM rule_objectgroup_refs WHERE rule_id = $1;

-- name: ListRuleIDsByObjectGroup :many
SELECT DISTINCT rule_id FROM rule_objectgroup_refs WHERE object_group_id = $1;
