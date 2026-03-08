-- name: ListLiveSnapshot :many
SELECT e.employee_code,
       e.name,
       d.name AS department_name,
       e.last_status,
       e.last_description,
       e.last_seen_at,
       CASE WHEN ws.active_start IS NULL THEN 0 ELSE 1 END AS is_working,
       COALESCE(tp.has_today_punch, 0) AS has_today_punch
FROM employees e
LEFT JOIN departments d ON e.department_id = d.id
LEFT JOIN (
  SELECT employee_id, MAX(start_at) AS active_start
  FROM work_sessions
  WHERE end_at IS NULL
  GROUP BY employee_id
) ws ON ws.employee_id = e.id
LEFT JOIN (
  SELECT employee_id,
         CASE WHEN COUNT(*) > 0 THEN 1 ELSE 0 END AS has_today_punch
  FROM work_sessions
  WHERE (start_at >= ?1 AND start_at < ?2)
     OR (end_at >= ?1 AND end_at < ?2)
  GROUP BY employee_id
) tp ON tp.employee_id = e.id
WHERE e.enabled = 1
ORDER BY e.id DESC;
