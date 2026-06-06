-- name: InsertConfigVersion :exec
INSERT INTO config_versions (tenant_id, entity_type, entity_id, version, content, created_by)
VALUES (
    $1, $2, $3,
    COALESCE((
        SELECT MAX(version) FROM config_versions
        WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3
    ), 0) + 1,
    $4, $5
);

-- name: ListConfigVersions :many
SELECT * FROM config_versions
WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3
ORDER BY version DESC;

-- name: GetConfigVersion :one
SELECT * FROM config_versions
WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3 AND version = $4;
