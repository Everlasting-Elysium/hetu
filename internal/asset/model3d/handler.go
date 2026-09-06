// Package model3d implements kernel.AssetHandler for standard 3D model
// exchange formats. Metadata extraction is trivial (3D files carry no raster
// dimensions); thumbnails are rendered out-of-process by a Blender headless
// sidecar over HTTP. When no sidecar is configured, thumbnailing degrades
// gracefully by returning domain.ErrNoThumbnail so a scan never blocks on 3D
// assets. ZBrush native formats (.ztl/.zpr) are opaque assets handled
// elsewhere, not here.
package model3d

import (
	"context"
	"io"
	"log/slog"

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

// Handler processes standard 3D model formats.
type Handler struct {
	blenderAddr string
}

var _ kernel.AssetHandler = (*Handler)(nil)

// New returns a 3D model handler. blenderAddr is the host:port of the Blender
// sidecar; an empty value disables thumbnail rendering (graceful degradation).
func New(blenderAddr string) *Handler {
	return &Handler{blenderAddr: blenderAddr}
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

// Thumbnail renders a PNG thumbnail via the Blender sidecar. With no sidecar
// configured it returns domain.ErrNoThumbnail so the indexer records the asset
// without a thumbnail instead of failing the scan.
func (h *Handler) Thumbnail(ctx context.Context, src io.ReadSeeker, w io.Writer) error {
	if h.blenderAddr == "" {
		slog.Warn("model3d: no blender sidecar configured; skipping 3D thumbnail")
		return domain.ErrNoThumbnail
	}
	return renderThumbnail(ctx, h.blenderAddr, src, w)
}
