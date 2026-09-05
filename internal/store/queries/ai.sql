-- name: GetTagIDByName :one
-- Resolves an owner's tag id by name so the AI pipeline can reuse an existing
-- (possibly manual) tag instead of creating a duplicate. Returns sql.ErrNoRows
-- when absent, signalling the caller to create it.
SELECT id FROM tags WHERE owner_id = ? AND name = ?;

-- name: ClearAIAssetTags :exec
-- Removes only AI-sourced tag links for the owner's assets. Manual links
-- (source='manual') are left intact so the ai layer is separately clearable
-- without touching user data (see docs/ai-and-3d.md).
DELETE FROM asset_tags
WHERE asset_tags.source = 'ai'
  AND EXISTS (SELECT 1 FROM assets WHERE assets.id = asset_tags.asset_id AND assets.owner_id = ?);

-- name: ClearAIAnnotations :exec
-- Removes only ai-layer annotations for the owner's assets. Manual and
-- extracted layers are never touched.
DELETE FROM annotations
WHERE annotations.layer = 'ai'
  AND EXISTS (SELECT 1 FROM assets WHERE assets.id = annotations.asset_id AND assets.owner_id = ?);
