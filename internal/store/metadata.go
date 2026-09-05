package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// IndexMetadata persists extracted file-embedded metadata (EXIF/IPTC/XMP) as
// extracted-layer annotations and updates asset.created_at when the metadata
// contains an embedded capture time. All writes share a transaction so a
// re-scan never leaves a half-updated index.
func (s *SQLite) IndexMetadata(ctx context.Context, owner domain.OwnerID, provider, path string, md domain.ExtractedMetadata) error {
	if len(md.Annotations) == 0 {
		return nil
	}
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin metadata tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	id, err := qtx.AssetIDByPath(ctx, db.AssetIDByPathParams{
		OwnerID: owner.String(), Provider: provider, StoragePath: path,
	})
	if err != nil {
		return fmt.Errorf("resolve asset %q: %w", path, err)
	}
	if err := upsertMetadataAnnotations(ctx, qtx, id, md); err != nil {
		return err
	}
	if !md.DateTime.IsZero() {
		if err := qtx.UpdateAssetCreatedAt(ctx, db.UpdateAssetCreatedAtParams{
			CreatedAt:   md.DateTime.Unix(),
			OwnerID:     owner.String(),
			Provider:    provider,
			StoragePath: path,
		}); err != nil {
			return fmt.Errorf("update created_at: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit metadata: %w", err)
	}
	return nil
}

func upsertMetadataAnnotations(ctx context.Context, q *db.Queries, id string, md domain.ExtractedMetadata) error {
	now := time.Now().Unix()
	for key, val := range md.Annotations {
		valJSON, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("marshal annotation %q: %w", key, err)
		}
		if err := q.UpsertAnnotation(ctx, db.UpsertAnnotationParams{
			AssetID:   id,
			Layer:     string(domain.LayerExtracted),
			Key:       key,
			Value:     string(valJSON),
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("upsert annotation %q: %w", key, err)
		}
	}
	return nil
}
