-- name: CreateManualAdjustment :execresult
INSERT INTO manual_adjustments (
  employee_id,
  start_at,
  end_at,
  reason,
  note,
  operator_id,
  status
) VALUES (?, ?, ?, ?, ?, ?, 'active');

-- name: UpdateManualAdjustment :exec
UPDATE manual_adjustments
SET start_at = ?, end_at = ?, reason = ?, note = ?, updated_at = NOW()
WHERE id = ? AND status = 'active';

-- name: RevokeManualAdjustment :exec
UPDATE manual_adjustments
SET status = 'revoked', updated_at = NOW()
WHERE id = ? AND status = 'active';

-- name: GetManualAdjustment :one
SELECT id, employee_id, start_at, end_at, reason, note, operator_id, status, created_at, updated_at
FROM manual_adjustments
WHERE id = ?
LIMIT 1;

-- name: ListManualAdjustments :many
SELECT ma.id, ma.start_at, ma.end_at, ma.reason, ma.note, ma.operator_id, ma.status, ma.created_at,
       e.employee_code, e.name, d.name AS department_name
FROM manual_adjustments ma
JOIN employees e ON ma.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
WHERE ( ? = '' OR DATE(ma.start_at) = ? )
ORDER BY ma.created_at DESC;
