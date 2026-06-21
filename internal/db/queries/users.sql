-- name: CreateUser :one
INSERT INTO users (username, password_hash)
VALUES (?, ?)
RETURNING id, username, password_hash, created_at;

-- name: GetUserByUsername :one
SELECT id, username, password_hash, created_at
FROM users
WHERE username = ?;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = ?
WHERE id = ?;
