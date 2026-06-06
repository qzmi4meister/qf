-- name: GetObjectGroup :one
SELECT * FROM object_groups WHERE id = $1 AND tenant_id = $2;

-- name: GetObjectGroupByName :one
SELECT * FROM object_groups WHERE tenant_id = $1 AND name = $2;

-- name: ListObjectGroups :many
SELECT * FROM object_groups WHERE tenant_id = $1 ORDER BY name;

-- name: ListObjectGroupsByType :many
SELECT * FROM object_groups WHERE tenant_id = $1 AND type = $2 ORDER BY name;

-- name: CreateObjectGroup :one
INSERT INTO object_groups (tenant_id, type, name, spec)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateObjectGroup :one
UPDATE object_groups
SET spec = $3, resolved_cidrs = NULL, resolved_at = NULL, updated_at = NOW()
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: UpdateObjectGroupResolved :exec
UPDATE object_groups
SET resolved_cidrs = $3, resolved_at = NOW(), updated_at = NOW()
WHERE id = $1 AND tenant_id = $2;

-- name: DeleteObjectGroup :exec
DELETE FROM object_groups WHERE id = $1 AND tenant_id = $2;
