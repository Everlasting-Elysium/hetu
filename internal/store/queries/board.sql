-- name: CreateBoard :exec
INSERT INTO boards (id, owner_id, name, created_at, updated_at)
VALUES (?, ?, ?, ?, ?);

-- name: ListBoards :many
SELECT id, owner_id, name, created_at, updated_at
FROM boards
WHERE owner_id = ?
ORDER BY updated_at DESC;

-- name: GetBoard :one
SELECT id, owner_id, name, created_at, updated_at
FROM boards
WHERE id = ? AND owner_id = ?;

-- name: UpdateBoardName :exec
UPDATE boards SET name = ?, updated_at = ?
WHERE id = ? AND owner_id = ?;

-- name: DeleteBoard :exec
DELETE FROM boards WHERE id = ? AND owner_id = ?;

-- name: TouchBoard :exec
UPDATE boards SET updated_at = ? WHERE id = ?;

-- name: CreateBoardItem :one
INSERT INTO board_items (id, board_id, asset_id, x, y, w, h, rotation, z, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, board_id, asset_id, x, y, w, h, rotation, z, created_at;

-- name: ListBoardItems :many
SELECT id, board_id, asset_id, x, y, w, h, rotation, z, created_at
FROM board_items
WHERE board_id = ?
ORDER BY z ASC, created_at ASC;

-- name: DeleteBoardItem :exec
DELETE FROM board_items WHERE id = ? AND board_id = ?;

-- name: DeleteBoardItemsByBoard :exec
DELETE FROM board_items WHERE board_id = ?;
