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
         FROM time_segments ts FORCE INDEX (idx_time_segments_employee_end)
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
         FROM work_sessions ws FORCE INDEX (idx_work_sessions_employee_end)
         WHERE ws.employee_id = ds.employee_id
           AND ws.start_at < sqlc.arg(report_end)
           AND (ws.end_at IS NULL OR ws.end_at > sqlc.arg(day_start))
       ), 0) AS SIGNED) AS on_duty_seconds
FROM daily_stats ds
JOIN employees e ON ds.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
WHERE ds.stat_date = sqlc.arg(stat_date)
  AND (sqlc.arg(department_id_filter) = 0 OR e.department_id = sqlc.narg(department_id))
ORDER BY ds.attendance_seconds DESC;

-- name: ListDailyStatsForRankByDate :many
SELECT e.employee_code,
       e.name,
       d.name AS department_name,
       ds.fish_seconds,
       ds.attendance_seconds,
       ds.effective_seconds
FROM daily_stats ds
JOIN employees e ON ds.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
WHERE ds.stat_date = ?;

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
       CAST(COALESCE((
         SELECT SUM(TIMESTAMPDIFF(
           SECOND,
           GREATEST(ts.start_at, sqlc.arg(day_start)),
           LEAST(ts.end_at, sqlc.arg(report_end))
         ))
         FROM time_segments ts FORCE INDEX (idx_time_segments_employee_end)
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
         FROM work_sessions ws FORCE INDEX (idx_work_sessions_employee_end)
         WHERE ws.employee_id = ds.employee_id
           AND ws.start_at < sqlc.arg(report_end)
           AND (ws.end_at IS NULL OR ws.end_at > sqlc.arg(day_start))
       ), 0) AS SIGNED) AS on_duty_seconds,
       (
         SELECT MIN(ws_start.start_at)
         FROM work_sessions ws_start FORCE INDEX (idx_work_sessions_employee_start)
         WHERE ws_start.employee_id = ds.employee_id
           AND ws_start.start_at >= sqlc.arg(day_start)
           AND ws_start.start_at < sqlc.arg(day_end)
       ) AS first_start_at,
       (
         SELECT MAX(ws_end.end_at)
         FROM work_sessions ws_end FORCE INDEX (idx_work_sessions_employee_end)
         WHERE ws_end.employee_id = ds.employee_id
           AND ws_end.end_at >= sqlc.arg(day_start)
           AND ws_end.end_at < sqlc.arg(day_end)
       ) AS last_end_at
FROM daily_stats ds
JOIN employees e ON ds.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
WHERE ds.stat_date = sqlc.arg(stat_date)
  AND (sqlc.arg(department_id_filter) = 0 OR e.department_id = sqlc.narg(department_id))
ORDER BY ds.attendance_seconds DESC;
