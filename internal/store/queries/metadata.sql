-- name: UpdateAssetCreatedAt :exec
-- Updates asset.created_at when embedded metadata (EXIF) provides a capture
-- time that should take priority over the filesystem modification time.
-- Uses the natural key (owner_id, provider, storage_path) so the canonical
-- row is resolved even after a re-scan discarded a fresh id on upsert.
UPDATE assets SET created_at = ?
WHERE owner_id = ? AND provider = ? AND storage_path = ?;
