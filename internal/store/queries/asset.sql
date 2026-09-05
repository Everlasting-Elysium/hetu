-- name: UpsertAsset :exec
INSERT INTO assets (
    id, owner_id, kind, provider, storage_path, name, ext, size, hash,
    thumb_path, width, height, created_at, indexed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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

-- name: ListAssets :many
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at
FROM assets
WHERE owner_id = ?
ORDER BY indexed_at DESC
LIMIT ? OFFSET ?;
