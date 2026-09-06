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
-- thumb_path/width/height resolve to the current version (issue #58) via the
-- LEFT JOIN: current_version_id is '' for un-versioned assets so cv is NULL and
-- COALESCE falls back to the anchor's own values. storage_path/hash stay
-- anchored to the original indexed file (scan/dedup/relocate key off them).
SELECT a.id, a.owner_id, a.kind, a.provider, a.storage_path, a.name, a.ext, a.size, a.hash,
       COALESCE(cv.thumb_path, a.thumb_path) AS thumb_path,
       COALESCE(cv.width, a.width) AS width,
       COALESCE(cv.height, a.height) AS height,
       a.created_at, a.indexed_at,
       a.deleted_at, a.rating, a.color, a.display_name, a.folder_id, a.missing_at,
       a.current_version_id
FROM assets a
LEFT JOIN asset_versions cv ON cv.id = a.current_version_id
WHERE a.id = ? AND a.owner_id = ?;

-- name: GetAssetByPath :one
-- Resolves the canonical asset row by its natural key (owner, provider, path).
-- Needed after UpsertAsset because the ON CONFLICT clause keeps the existing
-- id and discards a freshly generated one, so callers that import/index a file
-- must re-resolve to attach tags/folders/ratings/annotations to the right row.
-- Mirrors GetAsset's version-aware projection (issue #58) so it returns db.Asset.
SELECT a.id, a.owner_id, a.kind, a.provider, a.storage_path, a.name, a.ext, a.size, a.hash,
       COALESCE(cv.thumb_path, a.thumb_path) AS thumb_path,
       COALESCE(cv.width, a.width) AS width,
       COALESCE(cv.height, a.height) AS height,
       a.created_at, a.indexed_at,
       a.deleted_at, a.rating, a.color, a.display_name, a.folder_id, a.missing_at,
       a.current_version_id
FROM assets a
LEFT JOIN asset_versions cv ON cv.id = a.current_version_id
WHERE a.owner_id = ? AND a.provider = ? AND a.storage_path = ?;

-- name: ListAssets :many
-- thumb_path/width/height resolve to the current version (see GetAsset).
SELECT a.id, a.owner_id, a.kind, a.provider, a.storage_path, a.name, a.ext, a.size, a.hash,
       COALESCE(cv.thumb_path, a.thumb_path) AS thumb_path,
       COALESCE(cv.width, a.width) AS width,
       COALESCE(cv.height, a.height) AS height,
       a.created_at, a.indexed_at,
       a.deleted_at, a.rating, a.color, a.display_name, a.folder_id, a.missing_at,
       a.current_version_id
FROM assets a
LEFT JOIN asset_versions cv ON cv.id = a.current_version_id
WHERE a.owner_id = ? AND a.deleted_at IS NULL
ORDER BY a.indexed_at DESC
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
       deleted_at, rating, color, display_name, folder_id, missing_at,
       current_version_id
FROM assets
WHERE owner_id = ? AND hash = ? AND deleted_at IS NULL
ORDER BY indexed_at ASC;

-- name: ListMissingAssets :many
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id, missing_at,
       current_version_id
FROM assets
WHERE owner_id = ? AND missing_at IS NOT NULL AND deleted_at IS NULL
ORDER BY missing_at DESC
LIMIT ? OFFSET ?;

-- name: ListLiveAssetsByProvider :many
-- Returns all live (non-trashed, non-missing) assets for a provider, used by
-- the missing-file detector to check which indexed paths still exist on disk.
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id, missing_at,
       current_version_id
FROM assets
WHERE owner_id = ? AND provider = ? AND deleted_at IS NULL AND missing_at IS NULL
ORDER BY storage_path ASC;

-- name: ListMissingAssetsByHash :many
-- Returns missing assets matching a given hash, oldest first (by created_at).
-- Used by hash-based auto-reconnect during scan.
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, display_name, folder_id, missing_at,
       current_version_id
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
