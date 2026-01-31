-- name: CountAdminUsers :one
SELECT COUNT(1)
FROM admin_users;

-- name: GetAdminUserByUsername :one
SELECT id, username, password_hash, display_name, created_at
FROM admin_users
WHERE username = ?
LIMIT 1;

-- name: CreateAdminUser :exec
INSERT INTO admin_users (username, password_hash, display_name)
VALUES (?, ?, ?);

-- name: CreateAdminSession :exec
INSERT INTO admin_sessions (token, admin_id, issued_at, expires_at, revoked, last_seen)
VALUES (?, ?, ?, ?, 0, ?);

-- name: GetAdminSession :one
SELECT token, admin_id, issued_at, expires_at, revoked, last_seen
FROM admin_sessions
WHERE token = ?
LIMIT 1;

-- name: UpdateAdminSessionLastSeen :exec
UPDATE admin_sessions
SET last_seen = ?
WHERE token = ?;

-- name: RevokeAdminSession :exec
UPDATE admin_sessions
SET revoked = 1
WHERE token = ?;
