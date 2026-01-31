-- name: GetSettings :one
SELECT id, idle_threshold_seconds, heartbeat_interval_seconds, offline_threshold_seconds, fish_ratio_warn_percent, update_policy, latest_version, update_url, updated_at
FROM settings
WHERE id = 1;

-- name: UpsertSettings :exec
INSERT INTO settings (
  id,
  idle_threshold_seconds,
  heartbeat_interval_seconds,
  offline_threshold_seconds,
  fish_ratio_warn_percent,
  update_policy,
  latest_version,
  update_url,
  updated_at
) VALUES (
  1, ?, ?, ?, ?, ?, ?, ?, NOW()
)
ON DUPLICATE KEY UPDATE
  idle_threshold_seconds = VALUES(idle_threshold_seconds),
  heartbeat_interval_seconds = VALUES(heartbeat_interval_seconds),
  offline_threshold_seconds = VALUES(offline_threshold_seconds),
  fish_ratio_warn_percent = VALUES(fish_ratio_warn_percent),
  update_policy = VALUES(update_policy),
  latest_version = VALUES(latest_version),
  update_url = VALUES(update_url),
  updated_at = NOW();
