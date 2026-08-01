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
       ds.effective_seconds,
       CAST(COALESCE((
         SELECT SUM(TIMESTAMPDIFF(
           SECOND,
           GREATEST(ts.start_at, sqlc.arg(day_start)),
           LEAST(ts.end_at, sqlc.arg(report_end))
         ))
         FROM time_segments ts
         WHERE ts.employee_id = ds.employee_id
           AND ts.status = 'break'
           AND ts.start_at < sqlc.arg(report_end)
           AND ts.end_at > sqlc.arg(day_start)
       ), 0) AS SIGNED) AS break_seconds,
       CAST(COALESCE((
         SELECT SUM(TIMESTAMPDIFF(
           SECOND,
           GREATEST(ws.start_at, sqlc.arg(day_start)),
           LEAST(COALESCE(ws.end_at, sqlc.arg(report_end)), sqlc.arg(report_end))
         ))
         FROM work_sessions ws
         WHERE ws.employee_id = ds.employee_id
           AND ws.start_at < sqlc.arg(report_end)
           AND COALESCE(ws.end_at, sqlc.arg(report_end)) > sqlc.arg(day_start)
       ), 0) AS SIGNED) AS on_duty_seconds
FROM daily_stats ds
JOIN employees e ON ds.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
WHERE ds.stat_date = sqlc.arg(stat_date)
  AND (sqlc.arg(department_id_filter) = 0 OR e.department_id = sqlc.narg(department_id))
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
       CAST(COALESCE(breaks.break_seconds, 0) AS SIGNED) AS break_seconds,
       CAST(COALESCE(on_duty.on_duty_seconds, 0) AS SIGNED) AS on_duty_seconds,
       ws_start.first_start_at,
       ws_end.last_end_at
FROM daily_stats ds
JOIN employees e ON ds.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
LEFT JOIN (
  SELECT ts.employee_id,
         SUM(TIMESTAMPDIFF(SECOND, GREATEST(ts.start_at, ?), LEAST(ts.end_at, ?))) AS break_seconds
  FROM time_segments ts
  WHERE ts.status = 'break'
    AND ts.start_at < ?
    AND ts.end_at > ?
  GROUP BY ts.employee_id
) breaks ON breaks.employee_id = ds.employee_id
LEFT JOIN (
  SELECT ws.employee_id,
         SUM(TIMESTAMPDIFF(SECOND, GREATEST(ws.start_at, ?), LEAST(COALESCE(ws.end_at, ?), ?))) AS on_duty_seconds
  FROM work_sessions ws
  WHERE ws.start_at < ?
    AND COALESCE(ws.end_at, ?) > ?
  GROUP BY ws.employee_id
) on_duty ON on_duty.employee_id = ds.employee_id
LEFT JOIN (
  SELECT ws.employee_id,
         MIN(ws.start_at) AS first_start_at
  FROM work_sessions ws
  WHERE ws.start_at >= ?
    AND ws.start_at < ?
  GROUP BY ws.employee_id
) ws_start ON ws_start.employee_id = ds.employee_id
LEFT JOIN (
  SELECT ws.employee_id,
         MAX(ws.end_at) AS last_end_at
  FROM work_sessions ws
  WHERE ws.end_at IS NOT NULL
    AND ws.end_at >= ?
    AND ws.end_at < ?
  GROUP BY ws.employee_id
) ws_end ON ws_end.employee_id = ds.employee_id
WHERE ds.stat_date = ?
  AND (? = 0 OR e.department_id = ?)
ORDER BY ds.attendance_seconds DESC;
