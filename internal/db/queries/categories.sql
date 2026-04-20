-- name: ListCategories :many
SELECT id, user_id, name, sort_order, created_at
FROM categories
WHERE user_id = ?
ORDER BY sort_order, name;

-- name: CreateCategory :one
INSERT INTO categories (user_id, name, sort_order)
VALUES (?, ?, ?)
RETURNING id, user_id, name, sort_order, created_at;
