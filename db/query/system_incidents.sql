-- name: CreateIncident :execresult
INSERT INTO system_incidents (start_at, end_at, reason, note)
VALUES (?, ?, ?, ?);

-- name: UpdateIncident :exec
UPDATE system_incidents
SET start_at = ?, end_at = ?, reason = ?, note = ?
WHERE id = ?;

-- name: DeleteIncident :exec
DELETE FROM system_incidents WHERE id = ?;

-- name: ListIncidents :many
SELECT id, start_at, end_at, reason, note, created_at
FROM system_incidents
WHERE ( ? = '' OR DATE(start_at) = ? )
ORDER BY start_at DESC;
