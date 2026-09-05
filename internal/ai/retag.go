package ai

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// retagPageSize bounds how many assets a single Retag page loads so a large
// library is processed with bounded memory.
const retagPageSize = 200

// RetagStore is the store slice a full-library retag needs: it lists the owner's
// live assets and persists AI results to the ai layer. Implemented by
// *store.SQLite.
type RetagStore interface {
	ListAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error)
	Persister
}

// Retag re-runs AI tagging over every live asset for owner, persisting each
// result to the ai layer non-destructively (tags -> asset_tags source='ai',
// caption -> annotations layer='ai'). It pages through the library so memory
// stays bounded; a per-asset sidecar 501 is a skip (counted separately) rather
// than a failure, while a genuine tagger/store error aborts and is returned.
// Re-running is idempotent — manual and extracted data are never touched. It
// returns the number of assets tagged (persisted) and skipped.
func Retag(ctx context.Context, tagger Tagger, store RetagStore, owner domain.OwnerID, log *slog.Logger) (tagged, skipped int, err error) {
	for offset := 0; ; offset += retagPageSize {
		assets, err := store.ListAssets(ctx, owner, retagPageSize, offset)
		if err != nil {
			return tagged, skipped, fmt.Errorf("retag list assets: %w", err)
		}
		for _, a := range assets {
			wasSkipped, err := tagAndPersist(ctx, tagger, store, a, log)
			if err != nil {
				return tagged, skipped, err
			}
			if wasSkipped {
				skipped++
			} else {
				tagged++
			}
		}
		if len(assets) < retagPageSize {
			break
		}
	}
	log.InfoContext(ctx, "ai retag complete",
		slog.Int("tagged", tagged), slog.Int("skipped", skipped))
	return tagged, skipped, nil
}
