package store

import (
	"context"
	"fmt"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// UpsertAnnotation writes a single layered annotation for an asset, keyed by
// (asset_id, layer, key). owner is reserved for multi-user scoping; the
// annotations table is addressed by asset id (the caller already resolved the
// asset under its owner). A zero CreatedAt defaults to now. Value is stored
// verbatim, so callers pass a JSON-serialized payload to match the extracted/ai
// writers (see IndexMetadata and PersistAITagResult).
func (s *SQLite) UpsertAnnotation(ctx context.Context, _ domain.OwnerID, a domain.Annotation) error {
	created := a.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	if err := s.q.UpsertAnnotation(ctx, db.UpsertAnnotationParams{
		AssetID:   a.AssetID.String(),
		Layer:     string(a.Layer),
		Key:       a.Key,
		Value:     a.Value,
		Model:     a.Model,
		CreatedAt: created.Unix(),
	}); err != nil {
		return fmt.Errorf("upsert annotation %q: %w", a.Key, err)
	}
	return nil
}
