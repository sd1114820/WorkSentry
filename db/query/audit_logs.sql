-- name: CreateAuditLog :exec
INSERT INTO audit_logs (operator_id, action, target_type, target_id, detail)
VALUES (?, ?, ?, ?, ?);

-- name: ListAuditLogs :many
SELECT id, operator_id, action, target_type, target_id, COALESCE(detail, JSON_OBJECT()) AS detail, created_at
FROM audit_logs
ORDER BY id DESC
LIMIT 200;

-- name: ListAuditLogsByRange :many
SELECT id, operator_id, action, target_type, target_id, COALESCE(detail, JSON_OBJECT()) AS detail, created_at
FROM audit_logs
WHERE created_at >= ?
  AND created_at < ?
ORDER BY created_at DESC, id DESC
LIMIT 200;
