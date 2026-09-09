-- name: CreateCollection :exec
INSERT INTO collections (id, owner_id, parent_id, name, cover)
VALUES (?, ?, ?, ?, ?);

-- name: GetCollection :one
-- Returns the collection with its RAW stored cover (empty when auto-derived), so
-- the edit path can distinguish an explicit override from the fallback. The
-- resolved effective cover is a read-only concern of ListCollectionsWithCover.
SELECT id, owner_id, parent_id, name, cover
FROM collections
WHERE id = ? AND owner_id = ?;

-- name: ListCollectionsWithCover :many
-- Resolves each collection's effective cover in one pass: the explicit cover
-- override when set, otherwise the lowest-ord member's asset_id, otherwise '' for
-- an empty collection. CAST(... AS TEXT) pins the CASE result to a Go string.
SELECT c.id, c.owner_id, c.parent_id, c.name,
    CAST(CASE
        WHEN c.cover != '' THEN c.cover
        ELSE COALESCE((SELECT ci.asset_id FROM collection_items ci WHERE ci.collection_id = c.id ORDER BY ci.ord ASC LIMIT 1), '')
    END AS TEXT) AS effective_cover
FROM collections c
WHERE c.owner_id = ?
ORDER BY c.name, c.id;

-- name: UpdateCollection :exec
UPDATE collections SET name = ?, parent_id = ?, cover = ?
WHERE id = ? AND owner_id = ?;

-- name: DeleteCollection :exec
DELETE FROM collections WHERE id = ? AND owner_id = ?;

-- name: AddCollectionItem :exec
-- Idempotent: re-adding an existing member updates its ord instead of failing.
INSERT INTO collection_items (collection_id, asset_id, ord)
VALUES (?, ?, ?)
ON CONFLICT(collection_id, asset_id) DO UPDATE SET ord = excluded.ord;

-- name: RemoveCollectionItem :exec
DELETE FROM collection_items WHERE collection_id = ? AND asset_id = ?;

-- name: ClearCollectionCoverIfMatches :exec
-- Clears an explicit cover override when the just-removed asset was it, so a
-- removed member's thumbnail can never linger as the collection's cover (the
-- effective-cover CASE in ListCollectionsWithCover has no membership check of
-- its own; this keeps "cover is empty or a current member" true on the write
-- side instead).
UPDATE collections SET cover = '' WHERE id = ? AND cover = ?;

-- name: SetCollectionItemOrd :exec
UPDATE collection_items SET ord = ? WHERE collection_id = ? AND asset_id = ?;

-- name: NextCollectionItemOrd :one
-- max(ord)+1 for the next appended member; 0 for an empty collection.
SELECT CAST(COALESCE(MAX(ord), -1) + 1 AS INTEGER) AS next_ord
FROM collection_items WHERE collection_id = ?;

-- name: IsCollectionMember :one
SELECT COUNT(*) AS n FROM collection_items WHERE collection_id = ? AND asset_id = ?;

-- name: ListCollectionItemAssetIDs :many
SELECT asset_id FROM collection_items WHERE collection_id = ? ORDER BY ord ASC;

-- name: ListCollectionItemsEnriched :many
-- Ordered membership joined to assets so kind/name/thumb are resolved in one
-- query (mirroring boards' enrichment, but as a JOIN instead of N+1 GetAsset).
SELECT ci.asset_id, ci.ord, a.kind, a.name, a.display_name, a.thumb_path
FROM collection_items ci
JOIN assets a ON a.id = ci.asset_id
WHERE ci.collection_id = ?
ORDER BY ci.ord ASC;

-- name: DeleteCollectionItemsByCollection :exec
DELETE FROM collection_items WHERE collection_id = ?;
