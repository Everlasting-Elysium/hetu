package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// Persister writes an AI tagging result to hetu's ai metadata layer,
// non-destructively (manual data always wins). It is implemented by
// *store.SQLite; the ai package depends only on this narrow slice so the
// pipeline and the store stay decoupled and the kernel.Store contract is
// untouched.
type Persister interface {
	PersistAITagResult(ctx context.Context, owner domain.OwnerID, assetID domain.AssetID, res domain.AITagResult) error
}

// toDomain parses a sidecar TagResult into the domain result persisted to the ai
// layer, projecting scored labels to their names (trimming/dedup happens in
// domain.NewAITagResult).
func toDomain(res TagResult) domain.AITagResult {
	names := make([]string, 0, len(res.Tags))
	for _, t := range res.Tags {
		names = append(names, t.Name)
	}
	return domain.NewAITagResult(names, res.Caption, res.Model)
}

// tagAndPersist tags one asset via the sidecar and persists the result to the ai
// layer. A sidecar 501 (the Phase 1 stub for /tag, wrapped as ErrNotImplemented)
// is a graceful skip — skipped=true, err=nil — not a failure; every other tagger
// or store error is returned wrapped. This is the shared core of both the
// event-driven job ([orchestrator.tagJob]) and the batch [Retag] pass.
func tagAndPersist(ctx context.Context, tagger Tagger, store Persister, asset domain.Asset, log *slog.Logger) (skipped bool, err error) {
	assetID := asset.ID.String()
	res, err := tagger.Tag(ctx, AssetRef{Ref: asset.StoragePath})
	if err != nil {
		if errors.Is(err, ErrNotImplemented) {
			log.InfoContext(ctx, "ai_tag skipped: sidecar capability not implemented",
				slog.String("asset", assetID))
			return true, nil
		}
		return false, fmt.Errorf("ai_tag %s: %w", assetID, err)
	}
	parsed := toDomain(res)
	if err := store.PersistAITagResult(ctx, asset.Owner, asset.ID, parsed); err != nil {
		return false, fmt.Errorf("ai_tag persist %s: %w", assetID, err)
	}
	log.InfoContext(ctx, "ai_tag persisted",
		slog.String("asset", assetID),
		slog.Int("tags", len(parsed.TagNames)),
		slog.Bool("caption", parsed.Caption != ""),
		slog.String("model", parsed.Model))
	return false, nil
}
