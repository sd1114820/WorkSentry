-- name: CreateTimeSegment :exec
INSERT INTO time_segments (
  employee_id,
  start_at,
  end_at,
  status,
  description,
  source
) VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdateManualSegment :exec
UPDATE time_segments
SET start_at = ?, end_at = ?, description = ?
WHERE employee_id = ?
  AND source = 'manual'
  AND start_at = ?
  AND end_at = ?;

-- name: DeleteManualSegment :exec
DELETE FROM time_segments
WHERE employee_id = ?
  AND source = 'manual'
  AND start_at = ?
  AND end_at = ?;

-- name: ListTimeSegmentsByEmployeeAndRange :many
SELECT id, employee_id, start_at, end_at, status, description, source
FROM time_segments
WHERE employee_id = ?
  AND start_at < ?
  AND end_at > ?
ORDER BY start_at;

-- name: ListOfflineSegmentsByEmployeeAndRange :many
SELECT id, employee_id, start_at, end_at, status, description, source
FROM time_segments
WHERE employee_id = ?
  AND status = 'offline'
  AND start_at < ?
  AND end_at > ?
ORDER BY start_at;

-- name: ListOfflineSegmentsByDate :many
SELECT ts.employee_id,
       e.employee_code,
       e.name,
       d.name AS department_name,
       ts.start_at,
       ts.end_at
FROM employees e
STRAIGHT_JOIN time_segments ts
  ON ts.employee_id = e.id
 AND ts.status = 'offline'
 AND ts.start_at < sqlc.arg(range_end)
 AND ts.end_at > sqlc.arg(range_start)
LEFT JOIN departments d ON e.department_id = d.id
WHERE (sqlc.arg(employee_code_filter) = '' OR e.employee_code = sqlc.arg(employee_code))
ORDER BY e.employee_code, ts.start_at;

-- name: CountNonOfflineSegmentsOverlap :one
SELECT COUNT(1)
FROM time_segments
WHERE employee_id = ?
  AND status != 'offline'
  AND start_at < ?
  AND end_at > ?;

-- name: CountOfflineSegmentsCover :one
SELECT COUNT(1)
FROM time_segments
WHERE employee_id = ?
  AND status = 'offline'
  AND start_at <= ?
  AND end_at >= ?;
