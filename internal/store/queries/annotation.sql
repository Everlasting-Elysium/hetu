-- name: UpsertAnnotation :exec
INSERT INTO annotations (asset_id, layer, "key", value, model, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(asset_id, layer, "key") DO UPDATE SET
    value      = excluded.value,
    model      = excluded.model,
    created_at = excluded.created_at;

-- name: ListPHashAnnotations :many
-- Returns all pHash annotations for the owner's live assets, joining through
-- assets to filter by owner and exclude trashed items.
SELECT an.asset_id, an.value
FROM annotations an
JOIN assets a ON a.id = an.asset_id
WHERE a.owner_id = ? AND a.deleted_at IS NULL
  AND an.layer = 'extracted' AND an."key" = 'phash';
