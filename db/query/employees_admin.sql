-- name: ListEmployeesAdmin :many
SELECT e.id, e.employee_code, e.name, e.department_id, d.name AS department_name, e.fingerprint_hash, e.enabled, e.last_seen_at,
       ws.last_start_at, ws.last_end_at
FROM employees e
LEFT JOIN departments d ON e.department_id = d.id
LEFT JOIN (
  SELECT w1.employee_id, w1.start_at AS last_start_at, w1.end_at AS last_end_at
  FROM work_sessions w1
  JOIN (
    SELECT employee_id, MAX(start_at) AS max_start
    FROM work_sessions
    GROUP BY employee_id
  ) w2 ON w1.employee_id = w2.employee_id AND w1.start_at = w2.max_start
) ws ON ws.employee_id = e.id
ORDER BY e.id DESC;

-- name: ListEmployeesAdminByKeyword :many
SELECT e.id, e.employee_code, e.name, e.department_id, d.name AS department_name, e.fingerprint_hash, e.enabled, e.last_seen_at,
       ws.last_start_at, ws.last_end_at
FROM employees e
LEFT JOIN departments d ON e.department_id = d.id
LEFT JOIN (
  SELECT w1.employee_id, w1.start_at AS last_start_at, w1.end_at AS last_end_at
  FROM work_sessions w1
  JOIN (
    SELECT employee_id, MAX(start_at) AS max_start
    FROM work_sessions
    GROUP BY employee_id
  ) w2 ON w1.employee_id = w2.employee_id AND w1.start_at = w2.max_start
) ws ON ws.employee_id = e.id
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
