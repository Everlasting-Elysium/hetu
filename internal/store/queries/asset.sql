-- name: UpsertAsset :exec
-- Re-indexing preserves user metadata: the ON CONFLICT clause updates only
-- index-derived fields, leaving rating/color/display_name/folder_id/deleted_at
-- (and thus trash state) and missing_at untouched.
INSERT INTO assets (
    id, owner_id, kind, provider, storage_path, name, ext, size, hash,
    thumb_path, width, height, created_at, indexed_at,
    deleted_at, rating, color, display_name, folder_id, missing_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
       deleted_at, rating, color, display_name, folder_id, missing_at
FROM assets
WHERE id = ? AND owner_id = ?;

-- name: ListAssets :many
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id, missing_at
FROM assets
WHERE owner_id = ? AND deleted_at IS NULL
ORDER BY indexed_at DESC
LIMIT ? OFFSET ?;

-- name: ListDuplicateHashes :many
-- Returns hashes that appear more than once among the owner's live assets.
SELECT hash, COUNT(*) AS cnt
FROM assets
WHERE owner_id = ? AND deleted_at IS NULL
GROUP BY hash
HAVING COUNT(*) > 1
ORDER BY cnt DESC
LIMIT ? OFFSET ?;

-- name: ListAssetsByHash :many
-- Returns all live assets with the given hash for the owner.
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id, missing_at
FROM assets
WHERE owner_id = ? AND hash = ? AND deleted_at IS NULL
ORDER BY indexed_at ASC;

-- name: ListMissingAssets :many
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id, missing_at
FROM assets
WHERE owner_id = ? AND missing_at IS NOT NULL AND deleted_at IS NULL
ORDER BY missing_at DESC
LIMIT ? OFFSET ?;

-- name: ListLiveAssetsByProvider :many
-- Returns all live (non-trashed, non-missing) assets for a provider, used by
-- the missing-file detector to check which indexed paths still exist on disk.
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id, missing_at
FROM assets
WHERE owner_id = ? AND provider = ? AND deleted_at IS NULL AND missing_at IS NULL
ORDER BY storage_path ASC;

-- name: ListMissingAssetsByHash :many
-- Returns missing assets matching a given hash, oldest first (by created_at).
-- Used by hash-based auto-reconnect during scan.
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id, missing_at
FROM assets
WHERE owner_id = ? AND hash = ? AND missing_at IS NOT NULL AND deleted_at IS NULL
ORDER BY created_at ASC
LIMIT 1;

-- name: RelocateAsset :exec
-- Updates the storage path (and optionally provider) of a single asset and
-- clears missing_at. Used for manual relocate and hash-based auto-reconnect.
UPDATE assets
SET storage_path = ?, provider = ?, missing_at = NULL
WHERE id = ? AND owner_id = ?;

-- name: RebaseAssets :exec
-- Batch-updates storage_path by replacing old_prefix with new_prefix for all
-- assets whose path starts with old_prefix, and clears missing_at.
UPDATE assets
SET storage_path = ? || SUBSTR(storage_path, LENGTH(?) + 1),
    missing_at = NULL
WHERE owner_id = ? AND provider = ? AND storage_path LIKE ? || '%'
    AND deleted_at IS NULL;
