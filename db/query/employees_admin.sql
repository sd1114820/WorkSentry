-- name: ListEmployeesAdmin :many
SELECT e.id, e.employee_code, e.name, e.department_id, d.name AS department_name, e.fingerprint_hash, e.enabled, e.last_seen_at
FROM employees e
LEFT JOIN departments d ON e.department_id = d.id
ORDER BY e.id DESC;

-- name: ListEmployeesAdminByKeyword :many
SELECT e.id, e.employee_code, e.name, e.department_id, d.name AS department_name, e.fingerprint_hash, e.enabled, e.last_seen_at
FROM employees e
LEFT JOIN departments d ON e.department_id = d.id
WHERE e.employee_code LIKE ? OR e.name LIKE ?
ORDER BY e.id DESC;


-- name: GetMaxAutoEmployeeCodeNumber :one
SELECT COALESCE(MAX(CAST(SUBSTRING(employee_code, 6) AS UNSIGNED)), 0) AS max_number
FROM employees
WHERE employee_code LIKE 'AUTO-%';

-- name: CreateEmployee :exec
INSERT INTO employees (employee_code, name, department_id, enabled)
VALUES (?, ?, ?, ?);

-- name: UpdateEmployee :exec
UPDATE employees
SET employee_code = ?, name = ?, department_id = ?, enabled = ?
WHERE id = ?;

-- name: UpdateEmployeeEnabled :exec
UPDATE employees
SET enabled = ?
WHERE id = ?;

-- name: ClearEmployeeFingerprint :exec
UPDATE employees
SET fingerprint_hash = NULL
WHERE id = ?;
