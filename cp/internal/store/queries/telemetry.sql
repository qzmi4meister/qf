-- =============================================================================
-- Log events
-- =============================================================================

-- name: InsertLogEventsBatch :copyfrom
INSERT INTO log_events (
    tenant_id, host_id, rule_id, policy_id,
    direction, action, protocol,
    src_ip, src_port, dst_ip, dst_port,
    packet_size, tcp_flags, ct_state, created_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9, $10, $11,
    $12, $13, $14, $15
);

-- name: ListLogEvents :many
SELECT id, host_id, rule_id, policy_id, direction, action, protocol,
       src_ip, src_port, dst_ip, dst_port, packet_size, tcp_flags, ct_state, created_at
FROM log_events
WHERE tenant_id = $1
  AND host_id = $2
  AND ($3::timestamptz IS NULL OR created_at >= $3)
  AND ($4::timestamptz IS NULL OR created_at < $4)
  AND ($5::uuid IS NULL OR rule_id = $5)
  AND ($6 = '' OR action = $6)
ORDER BY created_at DESC
LIMIT $7;

-- =============================================================================
-- Flow events
-- =============================================================================

-- name: InsertFlowEventsBatch :copyfrom
INSERT INTO flow_events (
    tenant_id, host_id, protocol,
    src_ip, src_port, dst_ip, dst_port,
    bytes_orig, bytes_reply, packets_orig, packets_reply,
    final_state, started_at, ended_at, created_at
) VALUES (
    $1, $2, $3,
    $4, $5, $6, $7,
    $8, $9, $10, $11,
    $12, $13, $14, $15
);

-- name: ListFlowEvents :many
SELECT id, host_id, protocol,
       src_ip, src_port, dst_ip, dst_port,
       bytes_orig, bytes_reply, packets_orig, packets_reply,
       final_state, started_at, ended_at, created_at
FROM flow_events
WHERE tenant_id = $1
  AND host_id = $2
  AND ($3::timestamptz IS NULL OR created_at >= $3)
  AND ($4::timestamptz IS NULL OR created_at < $4)
ORDER BY created_at DESC
LIMIT $5;

-- =============================================================================
-- Counter snapshots
-- =============================================================================

-- name: InsertCounterSnapshotsBatch :copyfrom
INSERT INTO counter_snapshots (
    tenant_id, host_id, rule_id, policy_id, packets, bytes, ts
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListCounterSnapshots :many
SELECT id, host_id, rule_id, policy_id, packets, bytes, ts
FROM counter_snapshots
WHERE tenant_id = $1
  AND host_id = $2
  AND ($3::uuid IS NULL OR rule_id = $3)
  AND ($4::timestamptz IS NULL OR ts >= $4)
  AND ($5::timestamptz IS NULL OR ts < $5)
ORDER BY ts DESC
LIMIT $6;

-- Returns the latest snapshot for every rule on a host. Used by REST
-- /counters/latest and dead-rule detection.
-- name: GetLatestCounterSnapshotsForHost :many
SELECT DISTINCT ON (rule_id)
    id, host_id, rule_id, policy_id, packets, bytes, ts
FROM counter_snapshots
WHERE tenant_id = $1 AND host_id = $2
ORDER BY rule_id, ts DESC;

-- =============================================================================
-- System events
-- =============================================================================

-- name: InsertSystemEvent :one
INSERT INTO system_events (tenant_id, host_id, type, severity, detail, attributes)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListSystemEvents :many
SELECT id, host_id, type, severity, detail, attributes, created_at
FROM system_events
WHERE tenant_id = $1
  AND host_id = $2
  AND ($3::timestamptz IS NULL OR created_at >= $3)
  AND ($4::timestamptz IS NULL OR created_at < $4)
ORDER BY created_at DESC
LIMIT $5;

-- Deletes system events older than the given cutoff. Used by partition manager
-- for the non-partitioned system_events table.
-- name: DeleteOldSystemEvents :exec
DELETE FROM system_events WHERE created_at < $1;

-- =============================================================================
-- Audit log
-- =============================================================================

-- name: InsertAuditLog :one
INSERT INTO audit_log (tenant_id, actor_type, actor_id, action, object_type, object_id, before, after)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListAuditLog :many
SELECT id, tenant_id, actor_type, actor_id, action, object_type, object_id, before, after, created_at
FROM audit_log
WHERE tenant_id = $1
  AND ($2 = '' OR actor_type = $2)
  AND ($3::uuid IS NULL OR actor_id = $3)
  AND ($4 = '' OR object_type = $4)
  AND ($5::uuid IS NULL OR object_id = $5)
  AND ($6::timestamptz IS NULL OR created_at >= $6)
  AND ($7::timestamptz IS NULL OR created_at < $7)
ORDER BY created_at DESC
LIMIT $8;
