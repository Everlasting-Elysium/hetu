-- name: CreateTag :exec
INSERT INTO tags (id, owner_id, parent_id, name, color)
VALUES (?, ?, ?, ?, ?);

-- name: ListTags :many
SELECT id, owner_id, parent_id, name, color
FROM tags
WHERE owner_id = ?
ORDER BY name;

-- name: GetTag :one
SELECT id, owner_id, parent_id, name, color
FROM tags
WHERE id = ? AND owner_id = ?;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = ? AND owner_id = ?;

-- name: ReparentTagChildren :exec
-- Promote a tag's direct children up one level to its own parent. Used by
-- MergeTags before the merged-away tag is deleted so no child is left with a
-- parent_id pointing at a now-deleted tag (a dangling ref). Safe even when the
-- merge target is itself a child of the merged-away tag: it is promoted like any
-- other child instead of pointing at the deleted parent or at itself.
UPDATE tags SET parent_id = sqlc.arg(new_parent_id)
WHERE parent_id = sqlc.arg(old_parent_id) AND owner_id = sqlc.arg(owner_id);

-- name: AddAssetTag :exec
INSERT INTO asset_tags (asset_id, tag_id, source)
VALUES (?, ?, ?)
ON CONFLICT(asset_id, tag_id) DO NOTHING;

-- name: BatchRemoveTags :exec
DELETE FROM asset_tags
WHERE asset_id IN (sqlc.slice('asset_ids')) AND tag_id = ?;

-- name: ReattachAssetTags :exec
-- Re-hang every asset carrying from_tag_id onto into_tag_id (global tag merge).
-- INSERT OR IGNORE relies on the asset_tags (asset_id, tag_id) primary key to
-- dedup: an asset already carrying both tags keeps its single into_tag_id row
-- instead of failing on the conflict. The leftover from_tag_id rows are removed
-- separately by DeleteAssetTagsByTag.
INSERT OR IGNORE INTO asset_tags (asset_id, tag_id, source)
SELECT src.asset_id, sqlc.arg(into_tag_id), src.source
FROM asset_tags AS src
WHERE src.tag_id = sqlc.arg(from_tag_id);

-- name: DeleteAssetTagsByTag :exec
DELETE FROM asset_tags WHERE tag_id = ?;

-- name: ReattachAssetTagsForAssets :exec
-- Subset variant of ReattachAssetTags for batch replace: re-hang from_tag_id
-- onto into_tag_id only on the given assets. Same (asset_id, tag_id) primary-key
-- dedup as the global merge. from_tag_id itself is never deleted here (assets
-- outside the subset may still use it); the caller removes it only within the
-- subset via BatchRemoveTags.
INSERT OR IGNORE INTO asset_tags (asset_id, tag_id, source)
SELECT src.asset_id, sqlc.arg(into_tag_id), src.source
FROM asset_tags AS src
WHERE src.tag_id = sqlc.arg(from_tag_id) AND src.asset_id IN (sqlc.slice('asset_ids'));

-- name: ListAssetTags :many
SELECT tags.id, tags.owner_id, tags.parent_id, tags.name, tags.color
FROM tags
JOIN asset_tags ON tags.id = asset_tags.tag_id
WHERE asset_tags.asset_id = ?
ORDER BY tags.name;
