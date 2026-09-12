-- name: DeleteAssetFramesByAsset :exec
DELETE FROM asset_frames WHERE asset_id = ?;

-- name: InsertAssetFrame :exec
INSERT INTO asset_frames (asset_id, owner_id, frame_no, storage_path, name)
VALUES (?, ?, ?, ?, ?);

-- name: ListAssetFrames :many
SELECT asset_id, frame_no, storage_path, name
FROM asset_frames
WHERE asset_id = ? AND owner_id = ?
ORDER BY frame_no ASC;
