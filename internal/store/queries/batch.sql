-- name: BatchUpdateRating :exec
UPDATE assets SET rating = ?
WHERE id IN (sqlc.slice('ids')) AND owner_id = ? AND deleted_at IS NULL;

-- name: BatchUpdateColor :exec
UPDATE assets SET color = ?
WHERE id IN (sqlc.slice('ids')) AND owner_id = ? AND deleted_at IS NULL;

-- name: BatchUpdateDisplayName :exec
UPDATE assets SET display_name = ?
WHERE id IN (sqlc.slice('ids')) AND owner_id = ? AND deleted_at IS NULL;

-- name: SetDisplayName :exec
UPDATE assets SET display_name = ?
WHERE id = ? AND owner_id = ? AND deleted_at IS NULL;

-- name: BatchMoveToFolder :exec
UPDATE assets SET folder_id = ?
WHERE id IN (sqlc.slice('ids')) AND owner_id = ? AND deleted_at IS NULL;

-- name: BatchTrash :exec
UPDATE assets SET deleted_at = ?
WHERE id IN (sqlc.slice('ids')) AND owner_id = ? AND deleted_at IS NULL;

-- name: BatchRestore :exec
UPDATE assets SET deleted_at = NULL
WHERE id IN (sqlc.slice('ids')) AND owner_id = ? AND deleted_at IS NOT NULL;

-- name: PurgeTrash :exec
DELETE FROM assets
WHERE owner_id = ? AND deleted_at IS NOT NULL AND deleted_at < ?;

-- name: ListTrashedAssets :many
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id
FROM assets
WHERE owner_id = ? AND deleted_at IS NOT NULL
ORDER BY deleted_at DESC
LIMIT ? OFFSET ?;
