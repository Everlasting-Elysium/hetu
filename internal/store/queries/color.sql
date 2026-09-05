-- name: AssetIDByPath :one
SELECT id FROM assets
WHERE owner_id = ? AND provider = ? AND storage_path = ?;

-- name: DeleteAssetColors :exec
DELETE FROM asset_colors WHERE asset_id = ?;

-- name: InsertAssetColor :exec
INSERT INTO asset_colors (asset_id, owner_id, ord, hex, l, a, b, weight)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ColorCandidates :many
SELECT asset_id, hex, l, a, b FROM asset_colors
WHERE owner_id = ?;

-- name: AssetsByIDs :many
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at
FROM assets
WHERE owner_id = ? AND id IN (sqlc.slice('ids'));
