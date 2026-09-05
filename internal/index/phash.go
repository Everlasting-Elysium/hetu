package index

import (
	"context"
	"log/slog"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// indexPHash extracts and stores an asset's perceptual hash when the handler
// supports it (see kernel.PHashExtractor). Failures are logged and swallowed:
// pHash is an enhancement, so a decode or store error must not fail indexing of
// an otherwise-valid asset. The asset row must already be upserted so its id
// resolves in the store.
func (ix *Indexer) indexPHash(ctx context.Context, p kernel.StorageProvider, path string, h kernel.AssetHandler) {
	pe, ok := h.(kernel.PHashExtractor)
	if !ok {
		return
	}
	rc, err := p.Open(ctx, path)
	if err != nil {
		ix.warnPHash(ctx, "open", path, err)
		return
	}
	defer rc.Close()
	phash, err := pe.PHash(ctx, rc)
	if err != nil {
		ix.warnPHash(ctx, "extract", path, err)
		return
	}
	if err := ix.k.Store.IndexPHash(ctx, ix.owner, p.Name(), path, phash); err != nil {
		ix.warnPHash(ctx, "store", path, err)
	}
}

func (ix *Indexer) warnPHash(ctx context.Context, stage, path string, err error) {
	ix.k.Log.WarnContext(ctx, "phash "+stage,
		slog.String("path", path), slog.Any("err", err))
}
