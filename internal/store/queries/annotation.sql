-- name: UpsertAnnotation :exec
INSERT INTO annotations (asset_id, layer, "key", value, model, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(asset_id, layer, "key") DO UPDATE SET
    value      = excluded.value,
    model      = excluded.model,
    created_at = excluded.created_at;
