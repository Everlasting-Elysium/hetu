package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// assetColumns is the a-prefixed asset column list in db.Asset field order, so
// scanAssetRows and rowToAsset stay aligned with sqlc's generated queries.
// thumb_path/width/height COALESCE to the current version (issue #58); callers
// selecting these columns must join asset_versions as cv (see currentVersionJoin).
const assetColumns = `a.id, a.owner_id, a.kind, a.provider, a.storage_path, a.name, ` +
	`a.ext, a.size, a.hash, ` +
	`COALESCE(cv.thumb_path, a.thumb_path) AS thumb_path, ` +
	`COALESCE(cv.width, a.width) AS width, ` +
	`COALESCE(cv.height, a.height) AS height, a.created_at, ` +
	`a.indexed_at, a.deleted_at, a.rating, a.color, a.display_name, a.folder_id, ` +
	`a.missing_at, a.current_version_id`

// currentVersionJoin resolves an asset's current version for display-field
// COALESCE. current_version_id is '' for un-versioned assets, so cv is NULL and
// COALESCE falls back to the anchor's own thumb_path/width/height.
const currentVersionJoin = ` LEFT JOIN asset_versions cv ON cv.id = a.current_version_id `

// ListAssetsFiltered returns the owner's live assets narrowed by folder, tag,
// and minimum rating, newest first. Empty FolderID/TagID and MinRating 0 are
// ignored. Hand-written (not sqlc) because sqlc's SQLite engine cannot type
// optional filter params — the same reason SearchAssets is hand-written.
func (s *SQLite) ListAssetsFiltered(ctx context.Context, owner domain.OwnerID, f domain.AssetFilter, limit, offset int) ([]domain.Asset, error) {
	statusCond := "a.deleted_at IS NULL"
	if f.Status == "missing" {
		statusCond = "a.missing_at IS NOT NULL AND a.deleted_at IS NULL"
	}
	conds := []string{"a.owner_id = ?", statusCond}
	args := []any{owner.String()}
	if f.FolderID != "" {
		conds = append(conds, "a.folder_id = ?")
		args = append(args, f.FolderID)
	}
	if f.MinRating > 0 {
		conds = append(conds, "a.rating >= ?")
		args = append(args, f.MinRating)
	}
	if f.TagID != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM asset_tags atg WHERE atg.asset_id = a.id AND atg.tag_id = ?)")
		args = append(args, f.TagID)
	}
	query := "SELECT " + assetColumns + " FROM assets a" + currentVersionJoin + "WHERE " +
		strings.Join(conds, " AND ") + " ORDER BY a.indexed_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.sqldb.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list assets filtered: %w", err)
	}
	dbRows, err := scanAssetRows(rows)
	if err != nil {
		return nil, err
	}
	return rowsToAssets(dbRows)
}

// scanAssetRows scans rows selected with assetColumns into db.Asset values (the
// scan order must match assetColumns) and closes rows.
func scanAssetRows(rows *sql.Rows) ([]db.Asset, error) {
	defer func() { _ = rows.Close() }()
	var out []db.Asset
	for rows.Next() {
		var r db.Asset
		if err := rows.Scan(
			&r.ID, &r.OwnerID, &r.Kind, &r.Provider, &r.StoragePath, &r.Name,
			&r.Ext, &r.Size, &r.Hash, &r.ThumbPath, &r.Width, &r.Height,
			&r.CreatedAt, &r.IndexedAt, &r.DeletedAt, &r.Rating, &r.Color,
			&r.DisplayName, &r.FolderID, &r.MissingAt, &r.CurrentVersionID,
		); err != nil {
			return nil, fmt.Errorf("scan asset row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("asset rows: %w", err)
	}
	return out, nil
}
