package kernel

import (
	"context"
	"io"
	"strings"

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
