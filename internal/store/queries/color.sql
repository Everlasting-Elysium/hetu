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

-- name: GetAssetColors :many
SELECT ord, hex, weight FROM asset_colors
WHERE asset_id = ? AND owner_id = ?
ORDER BY ord ASC;

-- name: ListAssetColorsFull :many
-- Every swatch with its full Lab coordinates, ord-ascending. Used by the manual
-- delete path to re-insert the survivors with contiguous ords (0..N-1) without
-- recomputing Lab, and to locate the target ord before deleting.
SELECT ord, hex, l, a, b, weight FROM asset_colors
WHERE asset_id = ? AND owner_id = ?
ORDER BY ord ASC;

-- name: UpdateAssetColorAt :execrows
-- Repoints one swatch (by ord) at a new color; l/a/b are recomputed by the
-- caller. :execrows so a missing ord reports 0 rows -> the handler answers 404.
UPDATE asset_colors SET hex = ?, l = ?, a = ?, b = ?
WHERE asset_id = ? AND owner_id = ? AND ord = ?;

-- name: GetPaletteManual :one
-- The asset's palette_manual flag (0 = auto, 1 = user-curated). writePaletteTx
-- reads it to skip re-scan overwrites of a curated palette (issue #62).
SELECT palette_manual FROM assets WHERE id = ? AND owner_id = ?;

-- name: SetPaletteManual :exec
-- Flips an asset's palette_manual flag; set to 1 by every manual edit so a later
-- scan/thumb re-extract leaves the curated asset_colors rows untouched.
UPDATE assets SET palette_manual = ? WHERE id = ? AND owner_id = ?;

-- name: AssetsByIDs :many
-- Color-search / visual-similar results. thumb/dims stay the anchor's (not the
-- current version): the color and pHash indexes are built from the anchor at
-- scan time, so these discovery surfaces are anchor-scoped by construction.
SELECT id, owner_id, kind, provider, storage_path, name, ext, size, hash,
       thumb_path, width, height, created_at, indexed_at,
       deleted_at, rating, color, favorite, display_name, folder_id, missing_at,
       current_version_id, palette_manual
FROM assets
WHERE owner_id = ? AND id IN (sqlc.slice('ids'));
