-- name: AddDailyStats :exec
INSERT INTO daily_stats (
  stat_date,
  employee_id,
  work_seconds,
  normal_seconds,
  fish_seconds,
  idle_seconds,
  offline_seconds,
  attendance_seconds,
  effective_seconds
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  work_seconds = GREATEST(0, work_seconds + VALUES(work_seconds)),
  normal_seconds = GREATEST(0, normal_seconds + VALUES(normal_seconds)),
  fish_seconds = GREATEST(0, fish_seconds + VALUES(fish_seconds)),
  idle_seconds = GREATEST(0, idle_seconds + VALUES(idle_seconds)),
  offline_seconds = GREATEST(0, offline_seconds + VALUES(offline_seconds)),
  attendance_seconds = GREATEST(0, attendance_seconds + VALUES(attendance_seconds)),
  effective_seconds = GREATEST(0, effective_seconds + VALUES(effective_seconds));

-- name: ListDailyStatsByDate :many
SELECT ds.stat_date,
       e.employee_code,
       e.name,
       d.name AS department_name,
       ds.work_seconds,
       ds.normal_seconds,
       ds.fish_seconds,
       ds.idle_seconds,
       ds.offline_seconds,
       ds.attendance_seconds,
       ds.effective_seconds
FROM daily_stats ds
JOIN employees e ON ds.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
WHERE ds.stat_date = ?
  AND (? = 0 OR e.department_id = ?)
ORDER BY ds.attendance_seconds DESC;

-- name: ListDailyStatsForExportByDate :many
SELECT ds.stat_date,
       e.employee_code,
       e.name,
       d.name AS department_name,
       ds.work_seconds,
       ds.normal_seconds,
       ds.fish_seconds,
       ds.idle_seconds,
       ds.offline_seconds,
       ds.attendance_seconds,
       ds.effective_seconds,
       ws_start.first_start_at,
       ws_end.last_end_at
FROM daily_stats ds
JOIN employees e ON ds.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
LEFT JOIN (
  SELECT employee_id,
         MIN(start_at) AS first_start_at
  FROM work_sessions
  WHERE start_at >= ?
    AND start_at < ?
  GROUP BY employee_id
) ws_start ON ws_start.employee_id = ds.employee_id
LEFT JOIN (
  SELECT employee_id,
         MAX(end_at) AS last_end_at
  FROM work_sessions
  WHERE end_at IS NOT NULL
    AND end_at >= ?
    AND end_at < ?
  GROUP BY employee_id
) ws_end ON ws_end.employee_id = ds.employee_id
WHERE ds.stat_date = ?
  AND (? = 0 OR e.department_id = ?)
ORDER BY ds.attendance_seconds DESC;
