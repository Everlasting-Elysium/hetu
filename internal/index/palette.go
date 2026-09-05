package index

import (
	"context"
	"log/slog"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// indexPalette extracts and stores an asset's color palette when the handler
// supports it (see kernel.PaletteExtractor). Failures are logged and swallowed:
// color is an enhancement, so a decode or store error must not fail indexing of
// an otherwise-valid asset. The asset row must already be upserted so its id
// resolves in the store.
func (ix *Indexer) indexPalette(ctx context.Context, p kernel.StorageProvider, path string, h kernel.AssetHandler) {
	pe, ok := h.(kernel.PaletteExtractor)
	if !ok {
		return
	}
	rc, err := p.Open(ctx, path)
	if err != nil {
		ix.warnPalette(ctx, "open", path, err)
		return
	}
	defer rc.Close()
	pal, err := pe.Palette(ctx, rc)
	if err != nil {
		ix.warnPalette(ctx, "extract", path, err)
		return
	}
	if len(pal) == 0 {
		return
	}
	if err := ix.k.Store.IndexPalette(ctx, ix.owner, p.Name(), path, pal); err != nil {
		ix.warnPalette(ctx, "store", path, err)
	}
}

func (ix *Indexer) warnPalette(ctx context.Context, stage, path string, err error) {
	ix.k.Log.WarnContext(ctx, "palette "+stage,
		slog.String("path", path), slog.Any("err", err))
}
