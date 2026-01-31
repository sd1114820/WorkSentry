-- name: GetEmployeeByCode :one
SELECT id, employee_code, name, department_id, fingerprint_hash, enabled, last_seen_at, last_status, last_description, last_segment_end_at, created_at
FROM employees
WHERE employee_code = ?
LIMIT 1;

-- name: GetEmployeeByID :one
SELECT id, employee_code, name, department_id, fingerprint_hash, enabled, last_seen_at, last_status, last_description, last_segment_end_at, created_at
FROM employees
WHERE id = ?
LIMIT 1;

-- name: UpdateEmployeeFingerprint :exec
UPDATE employees
SET fingerprint_hash = ?
WHERE id = ?;

-- name: UpdateEmployeeLastSeen :exec
UPDATE employees
SET last_seen_at = ?, last_status = ?, last_description = ?
WHERE id = ?;

-- name: UpdateEmployeeLastSegmentEnd :exec
UPDATE employees
SET last_segment_end_at = ?
WHERE id = ?;

-- name: ListEmployees :many
SELECT id, employee_code, name, department_id, enabled
FROM employees
WHERE enabled = 1
ORDER BY id DESC;

-- name: ListEmployeesForOfflineRefresh :many
SELECT id, employee_code, name, department_id, fingerprint_hash, enabled, last_seen_at, last_status, last_description, last_segment_end_at, created_at
FROM employees
WHERE enabled = 1
  AND last_seen_at IS NOT NULL;
