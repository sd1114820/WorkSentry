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
