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
  SELECT ws.employee_id,
         CASE WHEN COUNT(*) > 0 THEN 1 ELSE 0 END AS has_today_punch
  FROM work_sessions ws
  WHERE (ws.start_at >= ? AND ws.start_at < ?)
     OR (ws.end_at >= ? AND ws.end_at < ?)
  GROUP BY ws.employee_id
) tp ON tp.employee_id = e.id
WHERE e.enabled = 1
ORDER BY e.id DESC;
