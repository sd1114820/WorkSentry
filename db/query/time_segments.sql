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
FROM time_segments ts
JOIN employees e ON ts.employee_id = e.id
LEFT JOIN departments d ON e.department_id = d.id
WHERE ts.status = 'offline'
  AND ts.start_at < ?
  AND ts.end_at > ?
ORDER BY ts.start_at;

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
