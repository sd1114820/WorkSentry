-- name: CreateAuditLog :exec
INSERT INTO audit_logs (operator_id, action, target_type, target_id, detail)
VALUES (?, ?, ?, ?, ?);

-- name: ListAuditLogs :many
SELECT id, operator_id, action, target_type, target_id, detail, created_at
FROM audit_logs
WHERE ( ? = '' OR DATE(created_at) = ? )
ORDER BY created_at DESC
LIMIT 200;
