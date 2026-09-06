-- name: CreateVersion :exec
INSERT INTO asset_versions (
    id, asset_id, owner_id, version_no, provider, storage_path, hash, size,
    thumb_path, width, height, note, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListVersions :many
-- All versions of an asset, newest version first.
SELECT id, asset_id, owner_id, version_no, provider, storage_path, hash, size,
       thumb_path, width, height, note, created_at
FROM asset_versions
WHERE asset_id = ? AND owner_id = ?
ORDER BY version_no DESC;

-- name: GetVersionByNo :one
-- One version addressed by (asset, version number); enforces ownership and that
-- the version belongs to the asset. Used by set-current and delete.
SELECT id, asset_id, owner_id, version_no, provider, storage_path, hash, size,
       thumb_path, width, height, note, created_at
FROM asset_versions
WHERE asset_id = ? AND owner_id = ? AND version_no = ?;

-- name: GetVersionByID :one
-- One version addressed by its id (owner-scoped). Used by the /file endpoint to
-- resolve the current version's bytes for playback/download.
SELECT id, asset_id, owner_id, version_no, provider, storage_path, hash, size,
       thumb_path, width, height, note, created_at
FROM asset_versions
WHERE id = ? AND owner_id = ?;

-- name: MaxVersionNo :one
-- Highest allocated version number for an asset (0 when none). CAST forces an
-- int64 return so version-number allocation is MaxVersionNo + 1.
SELECT CAST(COALESCE(MAX(version_no), 0) AS INTEGER) AS max_no
FROM asset_versions
WHERE asset_id = ?;

-- name: CountVersion :one
-- Existence probe for a version by identity. Used inside SetCurrentVersion's
-- transaction so a concurrent delete cannot leave current_version_id dangling.
SELECT COUNT(*) FROM asset_versions
WHERE id = ? AND asset_id = ? AND owner_id = ?;

-- name: GetAssetCurrentVersion :one
-- The asset's current-version pointer ('' when the asset has no explicit
-- versions yet; the anchor row itself is the implicit single version).
SELECT current_version_id FROM assets
WHERE id = ? AND owner_id = ?;

-- name: SetAssetCurrentVersion :exec
UPDATE assets SET current_version_id = ?
WHERE id = ? AND owner_id = ?;

-- name: DeleteVersion :exec
DELETE FROM asset_versions
WHERE id = ? AND asset_id = ? AND owner_id = ?;
