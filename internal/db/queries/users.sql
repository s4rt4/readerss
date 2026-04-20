-- name: CreateUser :one
INSERT INTO users (username, password_hash)
VALUES (?, ?)
RETURNING id, username, password_hash, created_at;

-- name: GetUserByUsername :one
SELECT id, username, password_hash, created_at
FROM users
WHERE username = ?;
