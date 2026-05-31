-- name: InsertCertificate :one
INSERT INTO certificates (tenant_id, host_id, serial, not_before, not_after, status)
VALUES ($1, $2, $3, $4, $5, 'active')
RETURNING *;

-- name: GetCertificateBySerial :one
SELECT * FROM certificates WHERE serial = $1;

-- name: GetActiveCertificateForHost :one
SELECT * FROM certificates
WHERE host_id = $1 AND status = 'active'
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateCertificateStatus :exec
UPDATE certificates
SET status = $2, revoked_at = CASE WHEN $2 = 'revoked' THEN NOW() ELSE NULL END
WHERE serial = $1;

-- name: ListRevokedActiveCerts :many
SELECT serial FROM certificates
WHERE status = 'revoked' AND not_after > NOW();
