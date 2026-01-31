-- name: CreateRawEvent :exec
INSERT INTO raw_events (
  employee_id,
  received_at,
  process_name,
  window_title,
  idle_seconds,
  status,
  client_version,
  ip_address
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetLastRawEventByEmployee :one
SELECT id, employee_id, received_at, process_name, window_title, idle_seconds, status, client_version, ip_address
FROM raw_events
WHERE employee_id = ?
ORDER BY received_at DESC
LIMIT 1;

-- name: DeleteRawEventsBefore :exec
DELETE FROM raw_events
WHERE received_at < ?;
