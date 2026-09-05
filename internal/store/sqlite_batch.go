package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// BatchUpdateRating sets rating (0-5) on all live assets in ids owned by owner.
func (s *SQLite) BatchUpdateRating(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID, rating int) error {
	if err := s.q.BatchUpdateRating(ctx, db.BatchUpdateRatingParams{
		Rating:  int64(rating),
		Ids:     idStrings(ids),
		OwnerID: owner.String(),
	}); err != nil {
		return fmt.Errorf("batch update rating: %w", err)
	}
	return nil
}

// BatchUpdateColor sets the color label on all live assets in ids.
func (s *SQLite) BatchUpdateColor(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID, color string) error {
	if err := s.q.BatchUpdateColor(ctx, db.BatchUpdateColorParams{
		Color:   color,
		Ids:     idStrings(ids),
		OwnerID: owner.String(),
	}); err != nil {
		return fmt.Errorf("batch update color: %w", err)
	}
	return nil
}

// BatchUpdateDisplayName sets the same display name on all live assets in ids.
func (s *SQLite) BatchUpdateDisplayName(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID, displayName string) error {
	if err := s.q.BatchUpdateDisplayName(ctx, db.BatchUpdateDisplayNameParams{
		DisplayName: displayName,
		Ids:         idStrings(ids),
		OwnerID:     owner.String(),
	}); err != nil {
		return fmt.Errorf("batch update display name: %w", err)
	}
	return nil
}

// BatchRenameDisplayNames applies a distinct display name per asset in one
// transaction, for pattern/sequence and find-replace renames.
func (s *SQLite) BatchRenameDisplayNames(ctx context.Context, owner domain.OwnerID, renames map[domain.AssetID]string) error {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rename tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)
	for id, name := range renames {
		if err := qtx.SetDisplayName(ctx, db.SetDisplayNameParams{
			DisplayName: name,
			ID:          id.String(),
			OwnerID:     owner.String(),
		}); err != nil {
			return fmt.Errorf("set display name %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rename tx: %w", err)
	}
	return nil
}

// BatchMoveToFolder reassigns folder_id (empty = root) on all live assets in ids.
func (s *SQLite) BatchMoveToFolder(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID, folderID string) error {
	if err := s.q.BatchMoveToFolder(ctx, db.BatchMoveToFolderParams{
		FolderID: folderID,
		Ids:      idStrings(ids),
		OwnerID:  owner.String(),
	}); err != nil {
		return fmt.Errorf("batch move to folder: %w", err)
	}
	return nil
}

// BatchTrashAssets soft-deletes the live assets in ids (sets deleted_at = now).
func (s *SQLite) BatchTrashAssets(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID) error {
	if err := s.q.BatchTrash(ctx, db.BatchTrashParams{
		DeletedAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true},
		Ids:       idStrings(ids),
		OwnerID:   owner.String(),
	}); err != nil {
		return fmt.Errorf("batch trash: %w", err)
	}
	return nil
}

// BatchRestoreAssets clears deleted_at on the trashed assets in ids.
func (s *SQLite) BatchRestoreAssets(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID) error {
	if err := s.q.BatchRestore(ctx, db.BatchRestoreParams{
		Ids:     idStrings(ids),
		OwnerID: owner.String(),
	}); err != nil {
		return fmt.Errorf("batch restore: %w", err)
	}
	return nil
}

// PurgeTrash permanently deletes trashed assets whose deleted_at is at or before
// retentionDays ago. retentionDays == 0 empties the trash entirely.
func (s *SQLite) PurgeTrash(ctx context.Context, owner domain.OwnerID, retentionDays int) error {
	// deleted_at is whole seconds; the query uses "< cutoff", so boundary+1
	// makes the comparison inclusive of the boundary second.
	boundary := time.Now().AddDate(0, 0, -retentionDays).Unix()
	if err := s.q.PurgeTrash(ctx, db.PurgeTrashParams{
		OwnerID:   owner.String(),
		DeletedAt: sql.NullInt64{Int64: boundary + 1, Valid: true},
	}); err != nil {
		return fmt.Errorf("purge trash: %w", err)
	}
	return nil
}

// ListTrashedAssets returns the owner's trashed assets, most recently trashed first.
func (s *SQLite) ListTrashedAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error) {
	rows, err := s.q.ListTrashedAssets(ctx, db.ListTrashedAssetsParams{
		OwnerID: owner.String(),
		Limit:   int64(limit),
		Offset:  int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list trashed assets: %w", err)
	}
	return rowsToAssets(rows)
}
