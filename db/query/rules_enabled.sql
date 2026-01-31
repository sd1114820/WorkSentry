-- name: ListEnabledRules :many
SELECT id, rule_type, match_mode, match_value, enabled, remark, created_at
FROM rules
WHERE enabled = 1
ORDER BY id DESC;
