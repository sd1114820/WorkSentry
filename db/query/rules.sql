-- name: ListRules :many
SELECT id, rule_type, match_mode, match_value, enabled, remark, created_at
FROM rules
ORDER BY id DESC;

-- name: CreateRule :execresult
INSERT INTO rules (rule_type, match_mode, match_value, enabled, remark)
VALUES (?, ?, ?, ?, ?);

-- name: UpdateRule :exec
UPDATE rules
SET rule_type = ?, match_mode = ?, match_value = ?, enabled = ?, remark = ?
WHERE id = ?;

-- name: DeleteRule :exec
DELETE FROM rules WHERE id = ?;
