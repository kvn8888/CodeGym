-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ?;

-- name: CreateUser :one
INSERT INTO users (id, username, display_name, password_hash, role)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateUser :exec
UPDATE users SET display_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;
