-- name: GetHost :one
SELECT * FROM hosts WHERE id = $1 AND tenant_id = $2;

-- name: ListHosts :many
SELECT * FROM hosts WHERE tenant_id = $1 ORDER BY created_at DESC;

-- name: ListHostsByStatus :many
SELECT * FROM hosts WHERE tenant_id = $1 AND status = $2 ORDER BY created_at DESC;

-- name: CreateHost :one
INSERT INTO hosts (tenant_id, hostname, labels, status)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateHostLabels :one
UPDATE hosts SET labels = $3, updated_at = NOW()
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: UpdateHostStatus :one
UPDATE hosts SET status = $3, updated_at = NOW()
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: UpdateHostHeartbeat :exec
UPDATE hosts
SET last_heartbeat_at = NOW(),
    current_generation = $3,
    agent_version = $4,
    kernel_version = $5,
    updated_at = NOW()
WHERE id = $1 AND tenant_id = $2;

-- name: UpdateHostGeneration :exec
UPDATE hosts SET current_generation = $3, updated_at = NOW()
WHERE id = $1 AND tenant_id = $2;

-- name: DeleteHost :exec
DELETE FROM hosts WHERE id = $1 AND tenant_id = $2;
