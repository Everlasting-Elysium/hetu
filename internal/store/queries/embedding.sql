-- name: UpsertEmbedding :exec
INSERT INTO embeddings (asset_id, embedding, model, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (asset_id) DO UPDATE SET
    embedding  = excluded.embedding,
    model      = excluded.model,
    created_at = excluded.created_at;

-- name: GetEmbedding :one
SELECT asset_id, embedding, model, created_at FROM embeddings WHERE asset_id = ?;

-- name: ListOwnerEmbeddings :many
SELECT e.asset_id, e.embedding
FROM embeddings e
JOIN assets a ON a.id = e.asset_id
WHERE a.owner_id = ? AND a.deleted_at IS NULL;
