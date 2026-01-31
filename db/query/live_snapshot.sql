-- name: ListLiveSnapshot :many
SELECT e.employee_code,
       e.name,
       d.name AS department_name,
       e.last_status,
       e.last_description,
       e.last_seen_at
FROM employees e
LEFT JOIN departments d ON e.department_id = d.id
WHERE e.enabled = 1
ORDER BY e.id DESC;
