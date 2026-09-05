-- name: EnsureOwner :exec
INSERT INTO users (id, created_at)
VALUES (?, ?)
ON CONFLICT(id) DO NOTHING;
