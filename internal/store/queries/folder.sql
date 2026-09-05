-- name: CreateFolder :exec
INSERT INTO folders (id, owner_id, parent_id, name, path)
VALUES (?, ?, ?, ?, ?);

-- name: ListFolders :many
SELECT id, owner_id, parent_id, name, path
FROM folders
WHERE owner_id = ?
ORDER BY path;

-- name: DeleteFolder :exec
DELETE FROM folders WHERE id = ? AND owner_id = ?;
