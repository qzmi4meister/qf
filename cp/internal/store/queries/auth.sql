-- ── Users ─────────────────────────────────────────────────────────────────

-- name: CreateUser :one
INSERT INTO users (tenant_id, email, password_hash, oidc_subject, status)
VALUES ($1, $2, $3, $4, 'active')
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1 AND tenant_id = $2;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE tenant_id = $1 AND email = $2;

-- name: ListUsers :many
SELECT * FROM users WHERE tenant_id = $1 ORDER BY created_at DESC;

-- name: UpdateUserPassword :one
UPDATE users SET password_hash = $3 WHERE id = $1 AND tenant_id = $2 RETURNING *;

-- name: UpdateUserStatus :one
UPDATE users SET status = $3 WHERE id = $1 AND tenant_id = $2 RETURNING *;

-- name: UpdateUserLastLogin :exec
UPDATE users SET last_login_at = NOW() WHERE id = $1;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1 AND tenant_id = $2;

-- ── User roles ────────────────────────────────────────────────────────────

-- name: GetUserRole :one
SELECT * FROM user_roles WHERE user_id = $1 AND tenant_id = $2;

-- name: UpsertUserRole :exec
INSERT INTO user_roles (user_id, tenant_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, tenant_id) DO UPDATE SET role = EXCLUDED.role;

-- name: DeleteUserRole :exec
DELETE FROM user_roles WHERE user_id = $1 AND tenant_id = $2;

-- ── API tokens ────────────────────────────────────────────────────────────

-- name: CreateAPIToken :one
INSERT INTO api_tokens (tenant_id, name, token_hash, role, created_by, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAPIToken :one
SELECT * FROM api_tokens WHERE id = $1 AND tenant_id = $2;

-- name: GetAPITokenByHash :one
SELECT * FROM api_tokens WHERE token_hash = $1;

-- name: ListAPITokens :many
SELECT * FROM api_tokens WHERE tenant_id = $1 ORDER BY created_at DESC;

-- name: DeleteAPIToken :exec
DELETE FROM api_tokens WHERE id = $1 AND tenant_id = $2;

-- name: UpdateAPITokenLastUsed :exec
UPDATE api_tokens SET last_used_at = NOW() WHERE id = $1;
