package kernel

import (
	"context"
	"io"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// AssetHandler processes one class of asset (image, video, 3D model, ...).
type AssetHandler interface {
	// Match reports whether this handler processes the given extension
	// (lowercase, without a leading dot).
	Match(ext string) bool
	// Kind is the asset kind this handler produces.
	Kind() domain.AssetKind
	// Extract reads src and returns extracted metadata.
	Extract(ctx context.Context, src io.ReadSeeker) (domain.Meta, error)
	// Thumbnail writes a thumbnail for src to w, or returns
	// domain.ErrNoThumbnail if it cannot produce one.
	Thumbnail(ctx context.Context, src io.ReadSeeker, w io.Writer) error
}

// PaletteExtractor is an optional AssetHandler capability. Handlers that can
// derive a color palette (currently images) implement it; the indexer uses a
// type assertion so handlers without color support are simply skipped.
type PaletteExtractor interface {
	Palette(ctx context.Context, src io.ReadSeeker) (color.Palette, error)
}

// PHashExtractor is an optional AssetHandler capability. Handlers that can
// compute a perceptual hash (currently images) implement it; the indexer uses
// a type assertion so handlers without pHash support are simply skipped.
type PHashExtractor interface {
	PHash(ctx context.Context, src io.ReadSeeker) (uint64, error)
}

// MetadataExtractor is an optional AssetHandler capability. Handlers that can
// extract embedded metadata (EXIF/IPTC/XMP for images) implement it; the
// indexer uses a type assertion so handlers without metadata support are
// simply skipped. Extracted values are stored as extracted-layer annotations.
type MetadataExtractor interface {
	ExtractMetadata(ctx context.Context, src io.ReadSeeker) (domain.ExtractedMetadata, error)
}

// PageExtractor is an optional AssetHandler capability for multi-page documents.
// Prepare readies src for paging exactly once — converting an office file to PDF,
// or copying a PDF, a single time — and returns a PagedDocument whose PageCount
// and RenderPage all reuse that one preparation, so a scan renders an N-page
// document with a single conversion rather than N. The indexer uses a type
// assertion, so handlers without page support are simply skipped. The caller
// must Close the returned document.
type PageExtractor interface {
	Prepare(ctx context.Context, src io.ReadSeeker) (PagedDocument, error)
}

// PagedDocument is a prepared multi-page document (a PDF on local disk, native or
// converted) ready to be counted and rendered page by page (1-based). Close
// releases the prepared file(s).
type PagedDocument interface {
	PageCount(ctx context.Context) (int, error)
	RenderPage(ctx context.Context, page int, w io.Writer) error
	Close() error
}

// AssetRegistry resolves an extension to the first matching handler.
type AssetRegistry struct {
	handlers []AssetHandler
}

// NewAssetRegistry returns an empty registry.
func NewAssetRegistry() *AssetRegistry { return &AssetRegistry{} }

// Register appends a handler. Order determines match precedence.
func (r *AssetRegistry) Register(h AssetHandler) {
	r.handlers = append(r.handlers, h)
}

// HandlerFor returns the first handler matching ext (any leading dot and case
// are ignored).
func (r *AssetRegistry) HandlerFor(ext string) (AssetHandler, bool) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	for _, h := range r.handlers {
		if h.Match(ext) {
			return h, true
		}
	}
	return nil, false
}
