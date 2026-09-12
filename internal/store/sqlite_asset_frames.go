package store

import (
	"context"
	"fmt"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// ReplaceAssetFrames rebuilds the frame index for the sequence asset identified
// by its natural key (owner, provider, path): it clears the asset's existing
// asset_frames rows and inserts the given frames in one transaction, so a
// re-scan whose frame count shrank never leaves stale rows. Resolving the
// canonical row id from the natural key (not a scan-time id) mirrors
// ReplaceDocumentPages: a re-scan generates a fresh id that UpsertAsset's ON
// CONFLICT discards, so only the anchor path resolves the durable id. An empty
// frames slice clears the index (the file is no longer part of a sequence).
func (s *SQLite) ReplaceAssetFrames(ctx context.Context, owner domain.OwnerID, provider, path string, frames []domain.AssetFrame) error {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin asset frames tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	id, err := qtx.AssetIDByPath(ctx, db.AssetIDByPathParams{
		OwnerID: owner.String(), Provider: provider, StoragePath: path,
	})
	if err != nil {
		return fmt.Errorf("resolve asset %q: %w", path, err)
	}
	if err := qtx.DeleteAssetFramesByAsset(ctx, id); err != nil {
		return fmt.Errorf("clear asset frames: %w", err)
	}
	for _, fr := range frames {
		if err := qtx.InsertAssetFrame(ctx, db.InsertAssetFrameParams{
			AssetID:     id,
			OwnerID:     owner.String(),
			FrameNo:     int64(fr.FrameNo),
			StoragePath: fr.StoragePath,
			Name:        fr.Name,
		}); err != nil {
			return fmt.Errorf("insert asset frame %d: %w", fr.FrameNo, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit asset frames: %w", err)
	}
	return nil
}

// ListAssetFrames returns the sequence asset's frames ordered by frame number.
// It returns an empty slice — never an error — when the asset has no frames, so
// the /frames endpoint can answer 200 with []. The owner filter scopes the read
// so a cross-owner asset id yields no frames.
func (s *SQLite) ListAssetFrames(ctx context.Context, owner domain.OwnerID, id domain.AssetID) ([]domain.AssetFrame, error) {
	rows, err := s.q.ListAssetFrames(ctx, db.ListAssetFramesParams{
		AssetID: id.String(), OwnerID: owner.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list asset frames %s: %w", id, err)
	}
	frames := make([]domain.AssetFrame, 0, len(rows))
	for _, r := range rows {
		aid, err := domain.NewAssetID(r.AssetID)
		if err != nil {
			return nil, fmt.Errorf("row asset frame id: %w", err)
		}
		frames = append(frames, domain.AssetFrame{
			AssetID:     aid,
			FrameNo:     int(r.FrameNo),
			StoragePath: r.StoragePath,
			Name:        r.Name,
		})
	}
	return frames, nil
}
