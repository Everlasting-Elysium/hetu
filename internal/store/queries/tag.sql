-- name: CreateTag :exec
INSERT INTO tags (id, owner_id, parent_id, name, color)
VALUES (?, ?, ?, ?, ?);

-- name: ListTags :many
SELECT id, owner_id, parent_id, name, color
FROM tags
WHERE owner_id = ?
ORDER BY name;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = ? AND owner_id = ?;

-- name: AddAssetTag :exec
INSERT INTO asset_tags (asset_id, tag_id, source)
VALUES (?, ?, ?)
ON CONFLICT(asset_id, tag_id) DO NOTHING;

-- name: BatchRemoveTags :exec
DELETE FROM asset_tags
WHERE asset_id IN (sqlc.slice('asset_ids')) AND tag_id = ?;

-- name: ListAssetTags :many
SELECT tags.id, tags.owner_id, tags.parent_id, tags.name, tags.color
FROM tags
JOIN asset_tags ON tags.id = asset_tags.tag_id
WHERE asset_tags.asset_id = ?
ORDER BY tags.name;
