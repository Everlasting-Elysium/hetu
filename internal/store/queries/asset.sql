-- name: UpsertAsset :exec
-- Re-indexing preserves user metadata: the ON CONFLICT clause updates only
-- index-derived fields, leaving rating/color/display_name/folder_id/deleted_at
-- (and thus trash state) untouched.
INSERT INTO assets (
    id, owner_id, kind, provider, storage_path, name, ext, size, hash,
    thumb_path, width, height, created_at, indexed_at,
    deleted_at, rating, color, display_name, folder_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(owner_id, provider, storage_path) DO UPDATE SET
    kind       = excluded.kind,
    name       = excluded.name,
    ext        = excluded.ext,
    size       = excluded.size,
    hash       = excluded.hash,
    thumb_path = excluded.thumb_path,
    width      = excluded.width,
    height     = excluded.height,
    indexed_at = excluded.indexed_at;

-- name: GetAsset :one
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id
FROM assets
WHERE id = ? AND owner_id = ?;

-- name: ListAssets :many
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id
FROM assets
WHERE owner_id = ? AND deleted_at IS NULL
ORDER BY indexed_at DESC
LIMIT ? OFFSET ?;
