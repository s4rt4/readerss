-- name: ListFilterRules :many
SELECT id, user_id, feed_id, match_type, pattern, action, created_at
FROM filter_rules
WHERE user_id = ?
ORDER BY created_at DESC, id DESC;

-- name: CreateFilterRule :one
INSERT INTO filter_rules (user_id, feed_id, match_type, pattern, action)
VALUES (?, ?, ?, ?, ?)
RETURNING id, user_id, feed_id, match_type, pattern, action, created_at;

-- name: DeleteFilterRule :exec
DELETE FROM filter_rules
WHERE id = ? AND user_id = ?;
