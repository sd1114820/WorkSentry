-- name: ListLiveSnapshot :many
SELECT e.employee_code,
       e.name,
       d.name AS department_name,
       e.last_status,
       e.last_description,
       e.last_seen_at,
       CASE WHEN ws.active_start IS NULL THEN 0 ELSE 1 END AS is_working
FROM employees e
LEFT JOIN departments d ON e.department_id = d.id
LEFT JOIN (
  SELECT employee_id, MAX(start_at) AS active_start
  FROM work_sessions
  WHERE end_at IS NULL
  GROUP BY employee_id
) ws ON ws.employee_id = e.id
WHERE e.enabled = 1
ORDER BY e.id DESC;
