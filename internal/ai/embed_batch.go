package ai

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// embedPageSize bounds how many assets a single EmbedAll page loads so a large
// library is processed with bounded memory.
const embedPageSize = 200

// EmbedAllStore is the store slice a full-library embed backfill needs: it lists
// the owner's live assets and persists embeddings. Implemented by *store.SQLite.
type EmbedAllStore interface {
	ListAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error)
	EmbedPersister
}

// EmbedAll generates and stores a CLIP embedding for every live asset of owner,
// paging through the library so memory stays bounded. It exists because
// `hetu scan` enqueues embedding jobs but runs no workers; this backfills a
// library indexed via the CLI. A per-asset sidecar 501 or empty vector is a skip
// (counted separately) rather than a failure; a genuine embedder/store error
// aborts and is returned. Re-running is idempotent (UpsertEmbedding). It returns
// the number of assets embedded and skipped.
func EmbedAll(ctx context.Context, embedder Embedder, store EmbedAllStore, owner domain.OwnerID, log *slog.Logger) (embedded, skipped int, err error) {
	for offset := 0; ; offset += embedPageSize {
		assets, err := store.ListAssets(ctx, owner, embedPageSize, offset)
		if err != nil {
			return embedded, skipped, fmt.Errorf("embed list assets: %w", err)
		}
		for _, a := range assets {
			wasSkipped, err := embedAndPersist(ctx, embedder, store, a, log)
			if err != nil {
				return embedded, skipped, err
			}
			if wasSkipped {
				skipped++
			} else {
				embedded++
			}
		}
		if len(assets) < embedPageSize {
			break
		}
	}
	log.InfoContext(ctx, "ai embed complete",
		slog.Int("embedded", embedded), slog.Int("skipped", skipped))
	return embedded, skipped, nil
}
