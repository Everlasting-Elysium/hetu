package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// searchAssetsSQL is hand-written because sqlc does not support FTS5 virtual
// tables. The query joins assets_fts (matched by MATCH) with assets via rowid,
// filters to the owner's live (non-trashed) assets, and orders by FTS5 bm25
// rank (ascending = best first). The column list mirrors db.Asset field order
// so the Scan below and rowToAsset stay aligned with sqlc's ListAssets.
// thumb_path/width/height resolve to the current version (issue #58) via the
// LEFT JOIN on current_version_id, so search results reflect the current
// revision's thumbnail; storage_path/hash stay anchored to the original.
const searchAssetsSQL = `
SELECT a.id, a.owner_id, a.kind, a.provider, a.storage_path, a.name, a.ext,
       a.size, a.hash,
       COALESCE(cv.thumb_path, a.thumb_path) AS thumb_path,
       COALESCE(cv.width, a.width) AS width,
       COALESCE(cv.height, a.height) AS height,
       a.created_at, a.indexed_at,
       a.deleted_at, a.rating, a.color, a.display_name, a.folder_id, a.missing_at,
       a.current_version_id
FROM assets_fts
JOIN assets a ON a.rowid = assets_fts.rowid
LEFT JOIN asset_versions cv ON cv.id = a.current_version_id
WHERE assets_fts MATCH ? AND a.owner_id = ? AND a.deleted_at IS NULL
ORDER BY assets_fts.rank
LIMIT ? OFFSET ?`

// SearchAssets performs FTS5 full-text search. ftsQuery must be a valid FTS5
// MATCH expression (produced by the search package parser).
func (s *SQLite) SearchAssets(ctx context.Context, owner domain.OwnerID, ftsQuery string, limit, offset int) ([]domain.Asset, error) {
	rows, err := s.sqldb.QueryContext(ctx, searchAssetsSQL, ftsQuery, owner.String(), limit, offset)
	if err != nil {
		if isFTSQueryError(err) {
			return nil, fmt.Errorf("%w: %s", domain.ErrInvalidQuery, err.Error())
		}
		return nil, fmt.Errorf("search assets: %w", err)
	}
	defer rows.Close()

	assets := []domain.Asset{}
	for rows.Next() {
		var r db.Asset
		if err := rows.Scan(
			&r.ID, &r.OwnerID, &r.Kind, &r.Provider, &r.StoragePath,
			&r.Name, &r.Ext, &r.Size, &r.Hash, &r.ThumbPath,
			&r.Width, &r.Height, &r.CreatedAt, &r.IndexedAt,
			&r.DeletedAt, &r.Rating, &r.Color, &r.DisplayName, &r.FolderID,
			&r.MissingAt, &r.CurrentVersionID,
		); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		a, err := rowToAsset(r)
		if err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search rows iteration: %w", err)
	}
	return assets, nil
}

// isFTSQueryError reports whether err is an FTS5 MATCH query error (malformed
// user query) rather than a server-side fault. modernc surfaces these as a
// generic SQLITE_ERROR, so detection is by message. The search package parser
// prevents these, so this is defense-in-depth against parser regressions.
func isFTSQueryError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "fts5: syntax error") ||
		strings.Contains(msg, "unknown special query")
}
