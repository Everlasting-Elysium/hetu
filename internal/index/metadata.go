package index

import (
	"context"
	"log/slog"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// indexMetadata extracts and stores an asset's embedded metadata (EXIF/IPTC/XMP)
// when the handler supports it (see kernel.MetadataExtractor). Failures are
// logged and swallowed: metadata is an enhancement, so a parse or store error
// must not fail indexing of an otherwise-valid asset. The asset row must already
// be upserted so its id resolves in the store.
func (ix *Indexer) indexMetadata(ctx context.Context, p kernel.StorageProvider, path string, h kernel.AssetHandler) {
	me, ok := h.(kernel.MetadataExtractor)
	if !ok {
		return
	}
	rc, err := p.Open(ctx, path)
	if err != nil {
		ix.warnMetadata(ctx, "open", path, err)
		return
	}
	defer rc.Close()
	md, err := me.ExtractMetadata(ctx, rc)
	if err != nil {
		ix.warnMetadata(ctx, "extract", path, err)
		return
	}
	if len(md.Annotations) == 0 {
		return
	}
	if err := ix.k.Store.IndexMetadata(ctx, ix.owner, p.Name(), path, md); err != nil {
		ix.warnMetadata(ctx, "store", path, err)
	}
}

func (ix *Indexer) warnMetadata(ctx context.Context, stage, path string, err error) {
	ix.k.Log.WarnContext(ctx, "metadata "+stage,
		slog.String("path", path), slog.Any("err", err))
}
