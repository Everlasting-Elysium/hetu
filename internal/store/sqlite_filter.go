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
	conds, args = appendFacetConds(conds, args, f)
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

// appendFacetConds appends the folder/rating/tag/kind plus size/dimension/shape
// narrowing conditions from f to conds (and their bind args to args), returning
// the extended slices. Size narrows the anchor a.size; width/height/shape narrow
// the current-version-resolved COALESCE(cv.*, a.*) (issue #101). It is
// shared by ListAssetsFiltered and SearchAssets so the sidebar facets narrow a
// plain listing and a keyword search identically. Lifecycle (Status) is the
// caller's concern — it differs, since search is always over live assets. Kinds
// are pre-validated against the AssetKind enum by the HTTP layer and every value
// is bound as a parameter, so a.kind IN (...) is injection-safe.
func appendFacetConds(conds []string, args []any, f domain.AssetFilter) ([]string, []any) {
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
	if len(f.Kinds) > 0 {
		ph := make([]string, len(f.Kinds))
		for i, k := range f.Kinds {
			ph[i] = "?"
			args = append(args, string(k))
		}
		conds = append(conds, "a.kind IN ("+strings.Join(ph, ",")+")")
	}
	// Size (bytes) narrows on the asset's own anchor row, a.size — NOT version-
	// resolved. hetu tracks no per-version file size, and a.size is also what
	// every read displays, so this stays consistent with the UI (issue #101
	// design decision 1; deliberately asymmetric with width/height below).
	if f.MinSize > 0 {
		conds = append(conds, "a.size >= ?")
		args = append(args, f.MinSize)
	}
	if f.MaxSize > 0 {
		conds = append(conds, "a.size <= ?")
		args = append(args, f.MaxSize)
	}
	// Pixel dimensions narrow on the CURRENT version's width/height (COALESCE
	// falls back to the anchor for un-versioned assets), matching the same
	// resolution already used for the displayed width/height column (#58).
	if f.MinWidth > 0 {
		conds = append(conds, "COALESCE(cv.width,a.width) >= ?")
		args = append(args, f.MinWidth)
	}
	if f.MaxWidth > 0 {
		conds = append(conds, "COALESCE(cv.width,a.width) <= ?")
		args = append(args, f.MaxWidth)
	}
	if f.MinHeight > 0 {
		conds = append(conds, "COALESCE(cv.height,a.height) >= ?")
		args = append(args, f.MinHeight)
	}
	if f.MaxHeight > 0 {
		conds = append(conds, "COALESCE(cv.height,a.height) <= ?")
		args = append(args, f.MaxHeight)
	}
	// Shape (aspect-ratio bucket) is a multi-select OR over the requested
	// buckets, current-version resolved like width/height above. The zero-
	// guard (excludes width=0 or height=0, e.g. audio/document/most 3D) is
	// shared once across every selected bucket rather than repeated per
	// bucket — (g&b1)|(g&b2) == g&(b1|b2), same result, simpler SQL.
	if len(f.Shapes) > 0 {
		var buckets []string
		for _, shp := range f.Shapes {
			switch shp {
			case domain.ShapeLandscape:
				buckets = append(buckets, "COALESCE(cv.width,a.width) >= ? * COALESCE(cv.height,a.height)")
				args = append(args, domain.ShapeLandscapeMinRatio)
			case domain.ShapePortrait:
				buckets = append(buckets, "COALESCE(cv.width,a.width) <= ? * COALESCE(cv.height,a.height)")
				args = append(args, domain.ShapePortraitMaxRatio)
			case domain.ShapeSquare:
				buckets = append(buckets, "(COALESCE(cv.width,a.width) > ? * COALESCE(cv.height,a.height) AND COALESCE(cv.width,a.width) < ? * COALESCE(cv.height,a.height))")
				args = append(args, domain.ShapePortraitMaxRatio, domain.ShapeLandscapeMinRatio)
			}
		}
		if len(buckets) > 0 {
			const shapeGuard = "COALESCE(cv.width,a.width) > 0 AND COALESCE(cv.height,a.height) > 0"
			conds = append(conds, "("+shapeGuard+") AND ("+strings.Join(buckets, " OR ")+")")
		}
	}
	return conds, args
}

// KindCounts returns the number of the owner's live assets of each kind,
// narrowed by f's folder/tag/rating but NOT by f.Kinds: the format facet needs a
// count for every format regardless of which formats are currently selected.
// Kinds absent from the (narrowed) library are absent from the map. It drives
// the sidebar/board format facet counts. It carries currentVersionJoin (the same
// 1:1 LEFT JOIN ListAssetsFiltered uses) so f's size/dimension/shape conditions
// can reference the cv alias; keyed on the version primary key it never inflates
// the per-kind counts.
func (s *SQLite) KindCounts(ctx context.Context, owner domain.OwnerID, f domain.AssetFilter) (map[domain.AssetKind]int, error) {
	f.Kinds = nil // counts span all formats regardless of the active kind facet
	conds := []string{"a.owner_id = ?", "a.deleted_at IS NULL"}
	args := []any{owner.String()}
	conds, args = appendFacetConds(conds, args, f)
	query := "SELECT a.kind, COUNT(*) FROM assets a" + currentVersionJoin + "WHERE " +
		strings.Join(conds, " AND ") + " GROUP BY a.kind"

	rows, err := s.sqldb.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("kind counts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[domain.AssetKind]int)
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			return nil, fmt.Errorf("scan kind count: %w", err)
		}
		out[domain.AssetKind(kind)] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kind count rows: %w", err)
	}
	return out, nil
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
