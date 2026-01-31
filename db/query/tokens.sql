-- name: CreateToken :exec
INSERT INTO client_tokens (token, employee_id, issued_at, expires_at, revoked, last_seen)
VALUES (?, ?, ?, ?, 0, ?);

-- name: GetToken :one
SELECT token, employee_id, issued_at, expires_at, revoked, last_seen
FROM client_tokens
WHERE token = ?
LIMIT 1;

-- name: UpdateTokenLastSeen :exec
UPDATE client_tokens
SET last_seen = ?
WHERE token = ?;

-- name: RevokeToken :exec
UPDATE client_tokens
SET revoked = 1
WHERE token = ?;

-- name: RevokeTokensByEmployee :exec
UPDATE client_tokens
SET revoked = 1
WHERE employee_id = ?;
