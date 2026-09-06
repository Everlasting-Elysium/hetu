package dam

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// mediaMIME maps lowercase extensions (no dot) to a Content-Type for the media
// formats hetu indexes. It keeps the /file endpoint deterministic regardless of
// the host's mime.types database, which minimal containers often lack entries
// for audio/video — without it the browser may refuse to play the stream.
var mediaMIME = map[string]string{
	// video (see internal/asset/video)
	"mp4": "video/mp4", "m4v": "video/mp4", "mov": "video/quicktime",
	"mkv": "video/x-matroska", "webm": "video/webm", "avi": "video/x-msvideo",
	// audio (see internal/asset/audio)
	"mp3": "audio/mpeg", "flac": "audio/flac", "wav": "audio/wav",
	"aac": "audio/aac", "ogg": "audio/ogg", "m4a": "audio/mp4",
	// image (view original)
	"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png",
	"gif": "image/gif", "webp": "image/webp", "bmp": "image/bmp",
	"tif": "image/tiff", "tiff": "image/tiff",
}

// contentType resolves a Content-Type from the asset extension, falling back to
// the stdlib mime table. An empty result lets http.ServeContent sniff the body.
func contentType(ext string) string {
	if ct, ok := mediaMIME[ext]; ok {
		return ct
	}
	return mime.TypeByExtension("." + ext)
}

// serveFile handles GET /assets/{id}/file: it streams the original asset bytes
// from the asset's storage provider. http.ServeContent adds Range support (HTTP
// 206 Partial Content) so <video>/<audio> can seek and play while downloading.
// Disposition is inline so media renders in-page; the SPA adds an explicit
// download affordance where it wants a save dialog. Scoped to the plugin owner,
// mirroring serveThumb — the storage path is never exposed to the client.
func (p *Plugin) serveFile(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	asset, err := p.k.Store.GetAsset(r.Context(), p.owner, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	// Serve the current version's bytes (issue #58): the asset's storage_path is
	// anchored to the original file, so resolve through current_version_id when
	// the asset is versioned. Falls back to the anchor for un-versioned assets.
	providerName, storagePath, ext := asset.Provider, asset.StoragePath, asset.Ext
	if v, ok := p.currentVersionFile(r.Context(), asset); ok {
		providerName, storagePath = v.Provider, v.StoragePath
		ext = strings.ToLower(strings.TrimPrefix(path.Ext(v.StoragePath), "."))
	}

	provider, ok := p.k.Storage.Get(providerName)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError,
			fmt.Errorf("storage provider %q not registered", providerName))
		return
	}
	info, err := provider.Stat(r.Context(), storagePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if info.IsDir {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("asset path is a directory"))
		return
	}
	f, err := provider.Open(r.Context(), storagePath)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("open asset: %w", err))
		return
	}
	defer f.Close()

	if ct := contentType(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	// Owner-scoped, so private; short TTL because the backing file can be
	// relocated/replaced (issue #45). ServeContent still revalidates via
	// Last-Modified/If-Range for Range requests.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", asset.Name))
	http.ServeContent(w, r, asset.Name, info.ModTime, f)
}

// currentVersionFile returns the asset's current version when it is explicitly
// versioned (issue #58), so callers serve the current revision's bytes. ok is
// false for an un-versioned asset (serve the anchor) or if the pointer cannot be
// resolved, in which case the caller falls back to the anchor.
func (p *Plugin) currentVersionFile(ctx context.Context, asset domain.Asset) (domain.AssetVersion, bool) {
	if asset.CurrentVersionID == "" {
		return domain.AssetVersion{}, false
	}
	vid, err := domain.NewVersionID(asset.CurrentVersionID)
	if err != nil {
		return domain.AssetVersion{}, false
	}
	v, err := p.k.Store.GetVersionByID(ctx, p.owner, vid)
	if err != nil {
		return domain.AssetVersion{}, false
	}
	return v, true
}
