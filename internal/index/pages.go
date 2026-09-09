package index

import (
	"context"
	"errors"
	"fmt"
	stdimage "image"
	_ "image/jpeg" // register the JPEG decoder so imageDims can read page thumbnails
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// maxDocumentPages caps how many pages the indexer renders for one document, so a
// malformed or hostile header reporting a huge page count (e.g. billions) cannot
// make a scan spin nearly forever. Documents past the cap are truncated to the
// first maxDocumentPages pages with a warning.
const maxDocumentPages = 500

// indexPages builds (or rebuilds) the per-page thumbnail index for a multi-page
// document (issue #48). Like the other index* enhancers it runs after upsert:
// failures are logged and swallowed so paging never fails an otherwise-valid
// asset, and handlers without page support (kernel.PageExtractor) are skipped.
//
// The document is prepared ONCE (converting an office file, or copying a PDF, a
// single time); the page count and every page render reuse that one preparation.
// A document with 0/1 pages (or a preparation/count failure) clears any rows a
// previous scan left and stops. Each page renders to {id}_p{N}.jpg under
// ThumbDir; a single page's failure is skipped rather than abandoning the rest,
// and the surviving pages replace the asset's rows wholesale so a shrunk page
// count never leaves stale rows.
func (ix *Indexer) indexPages(ctx context.Context, p kernel.StorageProvider, path, id string, h kernel.AssetHandler) {
	pe, ok := h.(kernel.PageExtractor)
	if !ok {
		return
	}
	doc, err := ix.prepareDoc(ctx, p, path, pe)
	if err != nil {
		ix.warnPages(ctx, "prepare", path, err)
		ix.clearPages(ctx, p, path)
		return
	}
	defer func() { _ = doc.Close() }()

	var count int
	err = guard(func() error {
		var e error
		count, e = doc.PageCount(ctx)
		return e
	})
	if err != nil {
		ix.warnPages(ctx, "count", path, err)
	}
	if err != nil || count <= 1 {
		ix.clearPages(ctx, p, path)
		return
	}
	if count > maxDocumentPages {
		ix.k.Log.WarnContext(ctx, "document page count capped",
			slog.String("path", path), slog.Int("reported", count), slog.Int("cap", maxDocumentPages))
		count = maxDocumentPages
	}
	if err := os.MkdirAll(ix.k.ThumbDir, 0o755); err != nil {
		ix.warnPages(ctx, "thumb dir", path, err)
		return
	}
	pages := ix.renderPages(ctx, doc, id, count)
	if err := ix.k.Store.ReplaceDocumentPages(ctx, ix.owner, p.Name(), path, pages); err != nil {
		ix.warnPages(ctx, "store", path, err)
	}
}

// prepareDoc opens the source and prepares it for paging exactly once; the source
// reader is closed as soon as preparation has captured its contents. The Prepare
// call is guarded so a decoder panic degrades to a skip.
func (ix *Indexer) prepareDoc(ctx context.Context, p kernel.StorageProvider, path string, pe kernel.PageExtractor) (kernel.PagedDocument, error) {
	rc, err := p.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var doc kernel.PagedDocument
	err = guard(func() error {
		var e error
		doc, e = pe.Prepare(ctx, rc)
		return e
	})
	return doc, err
}

// clearPages removes any page rows a previous scan left (page count dropped to
// 0/1, or the document is no longer renderable). Best-effort.
func (ix *Indexer) clearPages(ctx context.Context, p kernel.StorageProvider, path string) {
	if err := ix.k.Store.ReplaceDocumentPages(ctx, ix.owner, p.Name(), path, nil); err != nil {
		ix.warnPages(ctx, "clear", path, err)
	}
}

// renderPages renders pages 1..count from the prepared document to individual
// thumbnail files, returning those that rendered. It checks ctx before each page
// so a cancelled scan stops promptly (keeping already-rendered pages), and a
// single page's failure is skipped rather than abandoning the rest.
func (ix *Indexer) renderPages(ctx context.Context, doc kernel.PagedDocument, id string, count int) []domain.DocumentPage {
	pages := make([]domain.DocumentPage, 0, count)
	for page := 1; page <= count; page++ {
		if ctx.Err() != nil {
			break
		}
		thumbPath := filepath.Join(ix.k.ThumbDir, fmt.Sprintf("%s_p%d.jpg", id, page))
		w, h, err := ix.renderOnePage(ctx, doc, page, thumbPath)
		if err != nil {
			ix.warnPages(ctx, "render page "+strconv.Itoa(page), thumbPath, err)
			continue
		}
		pages = append(pages, domain.DocumentPage{PageNo: page, ThumbPath: thumbPath, Width: w, Height: h})
	}
	return pages
}

// renderOnePage renders a single page to thumbPath and returns its pixel
// dimensions, removing a partial file on failure so a later scan retries clean.
// The RenderPage call is guarded so a decoder panic degrades to a skipped page.
func (ix *Indexer) renderOnePage(ctx context.Context, doc kernel.PagedDocument, page int, thumbPath string) (int, int, error) {
	out, err := os.Create(thumbPath)
	if err != nil {
		return 0, 0, fmt.Errorf("create page thumb: %w", err)
	}
	genErr := guard(func() error { return doc.RenderPage(ctx, page, out) })
	closeErr := out.Close()
	if genErr != nil {
		_ = os.Remove(thumbPath)
		return 0, 0, genErr
	}
	if closeErr != nil {
		_ = os.Remove(thumbPath)
		return 0, 0, fmt.Errorf("close page thumb: %w", closeErr)
	}
	w, h := imageDims(thumbPath)
	return w, h, nil
}

// imageDims reads a rendered thumbnail's dimensions from its header. Zero on any
// failure — dimensions are a display convenience, never load-bearing.
func imageDims(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := stdimage.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// warnPages logs a paging failure, except an expected "no renderer"
// (domain.ErrNoThumbnail) which is the normal skip path, not a problem.
func (ix *Indexer) warnPages(ctx context.Context, stage, path string, err error) {
	if errors.Is(err, domain.ErrNoThumbnail) {
		return
	}
	ix.k.Log.WarnContext(ctx, "pages "+stage,
		slog.String("path", path), slog.Any("err", err))
}
