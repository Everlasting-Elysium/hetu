-- name: DeleteDocumentPagesByAsset :exec
DELETE FROM document_pages WHERE asset_id = ?;

-- name: InsertDocumentPage :exec
INSERT INTO document_pages (asset_id, owner_id, page_no, thumb_path, width, height)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListDocumentPages :many
SELECT asset_id, page_no, thumb_path, width, height
FROM document_pages
WHERE asset_id = ? AND owner_id = ?
ORDER BY page_no ASC;
