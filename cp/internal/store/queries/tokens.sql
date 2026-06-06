-- name: GetBootstrapTokenByHash :one
SELECT * FROM bootstrap_tokens
WHERE token_hash = $1 AND expires_at > NOW();

-- name: ListBootstrapTokens :many
SELECT * FROM bootstrap_tokens
WHERE tenant_id = $1 AND expires_at > NOW()
ORDER BY created_at DESC;

-- name: DeleteBootstrapToken :exec
DELETE FROM bootstrap_tokens WHERE id = $1 AND tenant_id = $2;

-- name: DeleteExpiredTokens :exec
DELETE FROM bootstrap_tokens WHERE expires_at <= NOW();
