// Package model3d implements kernel.AssetHandler for standard 3D model
// exchange formats. Metadata extraction is trivial (3D files carry no raster
// dimensions); thumbnails are produced client-side — the browser renders the
// model in <model-viewer> and uploads a screenshot (see dam.uploadThumb, issue
// #78) — so the handler never renders server-side and always reports
// domain.ErrNoThumbnail, letting a scan index a 3D asset without blocking on a
// preview. ZBrush native formats (.ztl/.zpr) are opaque assets handled
// elsewhere, not here.
package model3d

import (
	"context"
	"io"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// supported lists the standard 3D exchange formats hetu indexes and can render.
var supported = map[string]struct{}{
	"obj": {}, "fbx": {}, "glb": {}, "gltf": {},
	"stl": {}, "usd": {}, "usdz": {}, "ply": {},
}

// webFriendly lists the formats the web viewer (<model-viewer>) loads directly,
// with no server-side conversion. glTF/GLB are the browser-native 3D formats;
// every other supported format is converted to GLB before viewing (see
// ConvertToGLB). Note a .gltf with external buffers/textures only loads when
// self-contained (embedded/base64); split .gltf assets fall back in the UI.
var webFriendly = map[string]struct{}{
	"glb": {}, "gltf": {},
}

// Supported reports whether ext (lowercase, no leading dot) is a 3D exchange
// format hetu indexes. Used by the DAM viewer to reject opaque/native formats.
func Supported(ext string) bool {
	_, ok := supported[ext]
	return ok
}

// WebFriendly reports whether ext can be served to the browser as-is (glTF/GLB).
// Supported formats that are not web-friendly must be converted to GLB first.
func WebFriendly(ext string) bool {
	_, ok := webFriendly[ext]
	return ok
}

// Handler processes standard 3D model formats. It holds no state: 3D previews
// are rendered and captured client-side, so the handler only classifies formats.
type Handler struct{}

var _ kernel.AssetHandler = (*Handler)(nil)

// New returns a 3D model handler.
func New() *Handler {
	return &Handler{}
}

// Match reports whether ext is a supported 3D model extension.
func (h *Handler) Match(ext string) bool { return Supported(ext) }

// Kind returns domain.KindModel.
func (h *Handler) Kind() domain.AssetKind { return domain.KindModel }

// Extract returns model metadata. 3D formats carry no raster dimensions, so
// Width/Height stay zero and no file parsing is required.
func (h *Handler) Extract(_ context.Context, _ io.ReadSeeker) (domain.Meta, error) {
	return domain.Meta{Kind: domain.KindModel}, nil
}

// Thumbnail always reports domain.ErrNoThumbnail: hetu renders 3D previews in
// the browser (<model-viewer>) and stores a client-uploaded screenshot instead
// of rendering server-side (issue #78). The indexer records the asset without a
// preview; the thumbnail arrives later via POST /assets/{id}/thumb.
func (h *Handler) Thumbnail(_ context.Context, _ io.ReadSeeker, _ io.Writer) error {
	return domain.ErrNoThumbnail
}
