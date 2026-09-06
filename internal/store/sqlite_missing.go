package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// ListMissingAssets returns the owner's assets flagged missing (backing file
// absent from storage) that are not trashed, most recently flagged first.
func (s *SQLite) ListMissingAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error) {
	rows, err := s.q.ListMissingAssets(ctx, db.ListMissingAssetsParams{
		OwnerID: owner.String(),
		Limit:   int64(limit),
		Offset:  int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list missing assets: %w", err)
	}
	return rowsToAssets(rows)
}

// ListLiveAssetsByProvider returns the owner's live (non-trashed, non-missing)
// assets for a provider, ordered by storage path, for the missing-file detector.
func (s *SQLite) ListLiveAssetsByProvider(ctx context.Context, owner domain.OwnerID, provider string) ([]domain.Asset, error) {
	rows, err := s.q.ListLiveAssetsByProvider(ctx, db.ListLiveAssetsByProviderParams{
		OwnerID:  owner.String(),
		Provider: provider,
	})
	if err != nil {
		return nil, fmt.Errorf("list live assets by provider %s: %w", provider, err)
	}
	return rowsToAssets(rows)
}

// ListMissingAssetsByHash returns the owner's missing assets with the given
// hash, oldest first, for hash-based auto-reconnect (at most one row).
func (s *SQLite) ListMissingAssetsByHash(ctx context.Context, owner domain.OwnerID, hash string) ([]domain.Asset, error) {
	rows, err := s.q.ListMissingAssetsByHash(ctx, db.ListMissingAssetsByHashParams{
		OwnerID: owner.String(),
		Hash:    hash,
	})
	if err != nil {
		return nil, fmt.Errorf("list missing assets by hash: %w", err)
	}
	return rowsToAssets(rows)
}

// MarkAssetsMissing flags the live assets in ids as missing (missing_at = now).
func (s *SQLite) MarkAssetsMissing(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID) error {
	if err := s.q.MarkAssetsMissing(ctx, db.MarkAssetsMissingParams{
		MissingAt: sql.NullInt64{Int64: time.Now().Unix(), Valid: true},
		Ids:       idStrings(ids),
		OwnerID:   owner.String(),
	}); err != nil {
		return fmt.Errorf("mark assets missing: %w", err)
	}
	return nil
}

// MarkAssetsFound clears missing_at on the assets in ids (file re-appeared).
func (s *SQLite) MarkAssetsFound(ctx context.Context, owner domain.OwnerID, ids []domain.AssetID) error {
	if err := s.q.MarkAssetsFound(ctx, db.MarkAssetsFoundParams{
		Ids:     idStrings(ids),
		OwnerID: owner.String(),
	}); err != nil {
		return fmt.Errorf("mark assets found: %w", err)
	}
	return nil
}

// RelocateAsset points a single asset at a new provider/path and clears
// missing_at, for manual relocate and hash-based auto-reconnect.
func (s *SQLite) RelocateAsset(ctx context.Context, owner domain.OwnerID, id domain.AssetID, provider, newPath string) error {
	if err := s.q.RelocateAsset(ctx, db.RelocateAssetParams{
		StoragePath: newPath,
		Provider:    provider,
		ID:          id.String(),
		OwnerID:     owner.String(),
	}); err != nil {
		return fmt.Errorf("relocate asset %s: %w", id, err)
	}
	return nil
}

// RebaseAssets rewrites the storage-path prefix of every live asset under
// oldPrefix to newPrefix (provider-scoped) and clears missing_at, for
// folder-move recovery. The generated params carry SQLite's positional
// artifacts: LENGTH is the old prefix fed to LENGTH(?), Column5 the same prefix
// fed to the LIKE pattern.
func (s *SQLite) RebaseAssets(ctx context.Context, owner domain.OwnerID, provider, oldPrefix, newPrefix string) error {
	if err := s.q.RebaseAssets(ctx, db.RebaseAssetsParams{
		StoragePath: newPrefix,
		LENGTH:      oldPrefix,
		OwnerID:     owner.String(),
		Provider:    provider,
		Column5:     sql.NullString{String: oldPrefix, Valid: true},
	}); err != nil {
		return fmt.Errorf("rebase assets %s -> %s: %w", oldPrefix, newPrefix, err)
	}
	return nil
}
