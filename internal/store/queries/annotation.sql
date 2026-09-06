-- name: UpsertAnnotation :exec
INSERT INTO annotations (asset_id, layer, "key", value, model, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(asset_id, layer, "key") DO UPDATE SET
    value      = excluded.value,
    model      = excluded.model,
    created_at = excluded.created_at;

-- name: DeleteAnnotation :exec
DELETE FROM annotations
WHERE asset_id = ? AND layer = ? AND "key" = ?;

-- name: ListManualCaptions :many
-- Returns the manual-layer caption for each of the given owner's assets that
-- has one. Joins assets to enforce owner scoping.
SELECT an.asset_id, an.value
FROM annotations an
JOIN assets a ON a.id = an.asset_id
WHERE a.owner_id = ? AND an.asset_id IN (sqlc.slice('ids'))
  AND an.layer = 'manual' AND an."key" = 'caption';

-- name: ListPHashAnnotations :many
-- Returns all pHash annotations for the owner's live assets, joining through
-- assets to filter by owner and exclude trashed items.
SELECT an.asset_id, an.value
FROM annotations an
JOIN assets a ON a.id = an.asset_id
WHERE a.owner_id = ? AND a.deleted_at IS NULL
  AND an.layer = 'extracted' AND an."key" = 'phash';
