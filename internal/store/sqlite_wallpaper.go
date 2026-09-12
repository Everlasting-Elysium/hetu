package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// CountAssetsFiltered returns how many of the owner's assets match f, using the
// exact same status + facet conditions as ListAssetsFiltered (f.Kinds included,
// unlike KindCounts which ignores it). It backs the wallpaper daily-pick's
// deterministic index (issue #114): the handler needs the population size to
// compute a date-seeded offset. It carries currentVersionJoin AND durationJoin
// so f's dimension/shape/duration conditions resolve their cv/adur aliases,
// exactly like ListAssetsFiltered.
func (s *SQLite) CountAssetsFiltered(ctx context.Context, owner domain.OwnerID, f domain.AssetFilter) (int, error) {
	statusCond := "a.deleted_at IS NULL"
	if f.Status == "missing" {
		statusCond = "a.missing_at IS NOT NULL AND a.deleted_at IS NULL"
	}
	conds := []string{"a.owner_id = ?", statusCond}
	args := []any{owner.String()}
	conds, args = appendFacetConds(conds, args, f)
	query := "SELECT COUNT(*) FROM assets a" + currentVersionJoin + durationJoin + "WHERE " +
		strings.Join(conds, " AND ")

	var count int
	if err := s.sqldb.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count assets filtered: %w", err)
	}
	return count, nil
}

// ListCollectionAssets returns a collection's live member assets as full
// domain.Assets (current-version-resolved display fields via assetColumns +
// currentVersionJoin), ordered by the manual membership order (ci.ord), paged.
// The wallpaper collection view (issue #114) needs the width/height/size the
// lightweight domain.CollectionItem lacks, hence a dedicated query rather than
// reusing ListCollectionItems. It verifies the collection exists and belongs to
// owner first (returning domain.ErrNotFound otherwise) so a crafted id from
// another owner cannot enumerate assets.
func (s *SQLite) ListCollectionAssets(ctx context.Context, owner domain.OwnerID, collectionID domain.CollectionID, limit, offset int) ([]domain.Asset, error) {
	if _, err := s.GetCollection(ctx, owner, collectionID); err != nil {
		return nil, err
	}
	query := "SELECT " + assetColumns + " FROM collection_items ci JOIN assets a ON a.id = ci.asset_id" +
		currentVersionJoin + " WHERE ci.collection_id = ? AND a.owner_id = ? AND a.deleted_at IS NULL" +
		" ORDER BY ci.ord ASC LIMIT ? OFFSET ?"

	rows, err := s.sqldb.QueryContext(ctx, query, collectionID.String(), owner.String(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list collection assets: %w", err)
	}
	dbRows, err := scanAssetRows(rows)
	if err != nil {
		return nil, err
	}
	return rowsToAssets(dbRows)
}
