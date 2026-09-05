-- name: CreateShare :exec
INSERT INTO shares (
    id, owner_id, target_type, target_id, token,
    expires_at, password_hash, permission, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetShareByToken :one
SELECT id, owner_id, target_type, target_id, token,
       expires_at, password_hash, permission, created_at
FROM shares
WHERE token = ?;
