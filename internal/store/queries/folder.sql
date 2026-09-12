-- name: CreateFolder :exec
INSERT INTO folders (id, owner_id, parent_id, name, path, cover, color)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetFolder :one
-- Returns the folder with its RAW stored cover (empty when auto-derived), so the
-- edit path can distinguish an explicit override from the fallback. The resolved
-- effective cover is a read-only concern of ListFoldersWithCover.
SELECT id, owner_id, parent_id, name, path, cover, color
FROM folders
WHERE id = ? AND owner_id = ?;

-- name: ListFoldersWithCover :many
-- Resolves each folder's effective cover in one pass: the explicit cover override
-- when set, otherwise the folder's earliest-indexed live asset (folder_id match,
-- not trashed, oldest indexed_at first), otherwise empty for an empty folder.
-- CAST(... AS TEXT) pins the CASE result to a Go string.
SELECT f.id, f.owner_id, f.parent_id, f.name, f.path, f.color,
    CAST(CASE
        WHEN f.cover != '' THEN f.cover
        ELSE COALESCE((SELECT a.id FROM assets a WHERE a.folder_id = f.id AND a.deleted_at IS NULL ORDER BY a.indexed_at ASC, a.id ASC LIMIT 1), '')
    END AS TEXT) AS effective_cover
FROM folders f
WHERE f.owner_id = ?
ORDER BY f.path;

-- name: UpdateFolderCover :exec
UPDATE folders SET cover = ?, color = ?
WHERE id = ? AND owner_id = ?;

-- name: DeleteFolder :exec
DELETE FROM folders WHERE id = ? AND owner_id = ?;

-- name: CountOwnedLiveAsset :one
-- 1 when the asset id names a live (non-trashed) asset owned by owner, else 0.
-- Guards folder cover writes against dangling or cross-owner (IDOR) references.
SELECT COUNT(*) AS n FROM assets
WHERE id = ? AND owner_id = ? AND deleted_at IS NULL;
