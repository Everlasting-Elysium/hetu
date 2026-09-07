package index

import (
	"context"
	"log/slog"
	"os"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// indexPalette extracts and stores an asset's color palette. Failures are logged
// and swallowed: color is an enhancement, so a decode or store error must not
// fail indexing of an otherwise-valid asset. The asset row must already be
// upserted so its id resolves in the store.
//
// Two sources, by capability:
//   - Handlers that expose raster pixels directly (images) implement
//     kernel.PaletteExtractor and are read from the source file at full
//     resolution.
//   - Handlers whose source carries no raster (video, 3D) derive their palette
//     from the thumbnail the indexer just generated — the ffmpeg keyframe, an
//     optional Blender render, or a later client screenshot — so palette
//     extraction never triggers a second decode or render of its own.
func (ix *Indexer) indexPalette(ctx context.Context, p kernel.StorageProvider, path, thumbPath string, h kernel.AssetHandler) {
	pal, err := ix.extractPalette(ctx, p, path, thumbPath, h)
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

// extractPalette returns the asset's palette from the source (PaletteExtractor
// handlers) or from the generated thumbnail (everything else). A missing
// thumbnail yields a nil palette with no error: the asset simply has no color
// yet (e.g. a 3D model whose client screenshot has not been uploaded).
func (ix *Indexer) extractPalette(ctx context.Context, p kernel.StorageProvider, path, thumbPath string, h kernel.AssetHandler) (color.Palette, error) {
	if pe, ok := h.(kernel.PaletteExtractor); ok {
		rc, err := p.Open(ctx, path)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return pe.Palette(ctx, rc)
	}
	if thumbPath == "" {
		return nil, nil
	}
	f, err := os.Open(thumbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return color.ExtractPaletteFromReader(f)
}

func (ix *Indexer) warnPalette(ctx context.Context, stage, path string, err error) {
	ix.k.Log.WarnContext(ctx, "palette "+stage,
		slog.String("path", path), slog.Any("err", err))
}
