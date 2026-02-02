-- name: CreateWorkSession :exec
INSERT INTO work_sessions (
  employee_id,
  start_at,
  end_at
) VALUES (?, ?, NULL);

-- name: CloseWorkSession :exec
UPDATE work_sessions
SET end_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE employee_id = ?
  AND end_at IS NULL
ORDER BY start_at DESC
LIMIT 1;

-- name: GetOpenWorkSessionByEmployee :one
SELECT id, employee_id, start_at, end_at, created_at, updated_at
FROM work_sessions
WHERE employee_id = ?
  AND end_at IS NULL
ORDER BY start_at DESC
LIMIT 1;
