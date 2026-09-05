package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// tagSourceAI marks a tag link produced by the AI tagging pipeline (vs. a
// user's manual tag). It scopes the asset_tags.source column, which is cleared
// independently of manual links so the ai layer stays non-destructive.
const tagSourceAI = "ai"

// PersistAITagResult writes an AI tagging result to the ai layer for one asset,
// non-destructively and in a single transaction:
//   - each tag name is resolved to a tag row (created if absent) and linked in
//     asset_tags with source='ai'; an existing (manual) link is left untouched
//     by ON CONFLICT DO NOTHING, so manual always wins;
//   - a non-empty caption is upserted into annotations(layer='ai', key=caption)
//     carrying the producing model id.
//
// Manual and extracted rows are never modified. Re-running is idempotent: tag
// links dedupe on their primary key and the caption annotation upserts in place.
func (s *SQLite) PersistAITagResult(ctx context.Context, owner domain.OwnerID, assetID domain.AssetID, res domain.AITagResult) error {
	if len(res.TagNames) == 0 && res.Caption == "" {
		return nil
	}
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ai persist tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	for _, name := range res.TagNames {
		tagID, err := resolveTagID(ctx, qtx, owner, name)
		if err != nil {
			return err
		}
		if err := qtx.AddAssetTag(ctx, db.AddAssetTagParams{
			AssetID: assetID.String(), TagID: tagID, Source: tagSourceAI,
		}); err != nil {
			return fmt.Errorf("link ai tag %q: %w", name, err)
		}
	}
	if res.Caption != "" {
		valJSON, err := json.Marshal(res.Caption)
		if err != nil {
			return fmt.Errorf("marshal caption: %w", err)
		}
		if err := qtx.UpsertAnnotation(ctx, db.UpsertAnnotationParams{
			AssetID:   assetID.String(),
			Layer:     string(domain.LayerAI),
			Key:       domain.KeyCaption,
			Value:     string(valJSON),
			Model:     res.Model,
			CreatedAt: time.Now().Unix(),
		}); err != nil {
			return fmt.Errorf("upsert ai caption: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ai persist: %w", err)
	}
	return nil
}

// resolveTagID returns the id of the owner's tag named name, creating it when
// absent so an AI-discovered label reuses any existing (manual) tag rather than
// duplicating it.
func resolveTagID(ctx context.Context, q *db.Queries, owner domain.OwnerID, name string) (string, error) {
	id, err := q.GetTagIDByName(ctx, db.GetTagIDByNameParams{OwnerID: owner.String(), Name: name})
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("resolve tag %q: %w", name, err)
	}
	id = uuid.Must(uuid.NewV7()).String()
	if err := q.CreateTag(ctx, db.CreateTagParams{ID: id, OwnerID: owner.String(), Name: name}); err != nil {
		return "", fmt.Errorf("create ai tag %q: %w", name, err)
	}
	return id, nil
}

// ClearAILayer removes all AI-layer data for the owner in one transaction:
// asset_tags rows with source='ai' and annotations rows with layer='ai'. Manual
// and extracted rows are never touched, so the ai layer can be wiped and re-run
// (e.g. after a model upgrade) without disturbing user data.
func (s *SQLite) ClearAILayer(ctx context.Context, owner domain.OwnerID) error {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ai clear tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)
	if err := qtx.ClearAIAssetTags(ctx, owner.String()); err != nil {
		return fmt.Errorf("clear ai asset tags: %w", err)
	}
	if err := qtx.ClearAIAnnotations(ctx, owner.String()); err != nil {
		return fmt.Errorf("clear ai annotations: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ai clear: %w", err)
	}
	return nil
}
