package store

import (
	"context"
	"fmt"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// ReplaceDocumentPages rebuilds the per-page thumbnail index for the asset
// identified by its natural key (owner, provider, path): it clears the asset's
// existing document_pages rows and inserts the given pages in one transaction,
// so a re-scan whose page count shrank never leaves stale rows. Resolving the
// canonical row id from the natural key (not a scan-time id) mirrors
// IndexPalette/IndexMetadata: a re-scan generates a fresh id that UpsertAsset's
// ON CONFLICT discards, so only the path resolves the durable id. An empty
// pages slice clears the index (single-page or thumbnail-less document).
func (s *SQLite) ReplaceDocumentPages(ctx context.Context, owner domain.OwnerID, provider, path string, pages []domain.DocumentPage) error {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin document pages tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	id, err := qtx.AssetIDByPath(ctx, db.AssetIDByPathParams{
		OwnerID: owner.String(), Provider: provider, StoragePath: path,
	})
	if err != nil {
		return fmt.Errorf("resolve asset %q: %w", path, err)
	}
	if err := qtx.DeleteDocumentPagesByAsset(ctx, id); err != nil {
		return fmt.Errorf("clear document pages: %w", err)
	}
	for _, pg := range pages {
		if err := qtx.InsertDocumentPage(ctx, db.InsertDocumentPageParams{
			AssetID:   id,
			OwnerID:   owner.String(),
			PageNo:    int64(pg.PageNo),
			ThumbPath: pg.ThumbPath,
			Width:     int64(pg.Width),
			Height:    int64(pg.Height),
		}); err != nil {
			return fmt.Errorf("insert document page %d: %w", pg.PageNo, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit document pages: %w", err)
	}
	return nil
}

// ListDocumentPages returns the asset's rendered pages ordered by page number.
// It returns an empty slice — never an error — when the asset has no pages, so
// the /pages endpoint can answer 200 with []. The owner filter scopes the read
// so a cross-owner asset id yields no pages.
func (s *SQLite) ListDocumentPages(ctx context.Context, owner domain.OwnerID, id domain.AssetID) ([]domain.DocumentPage, error) {
	rows, err := s.q.ListDocumentPages(ctx, db.ListDocumentPagesParams{
		AssetID: id.String(), OwnerID: owner.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list document pages %s: %w", id, err)
	}
	pages := make([]domain.DocumentPage, 0, len(rows))
	for _, r := range rows {
		aid, err := domain.NewAssetID(r.AssetID)
		if err != nil {
			return nil, fmt.Errorf("row document page asset id: %w", err)
		}
		pages = append(pages, domain.DocumentPage{
			AssetID:   aid,
			PageNo:    int(r.PageNo),
			ThumbPath: r.ThumbPath,
			Width:     int(r.Width),
			Height:    int(r.Height),
		})
	}
	return pages, nil
}
