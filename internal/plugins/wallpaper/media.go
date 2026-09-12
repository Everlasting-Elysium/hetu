package wallpaper

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// imageVideoMIME maps lowercase extensions (no dot) to a Content-Type for the
// wallpaper-relevant formats (images + videos only). It is a deliberately
// smaller table than DAM's mediaMIME — wallpapers are never audio/document/3D —
// kept local so the plugin owns its own narrow concern; contentType falls back
// to the stdlib mime table for anything outside it.
var imageVideoMIME = map[string]string{
	"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png",
	"gif": "image/gif", "webp": "image/webp", "bmp": "image/bmp",
	"tif": "image/tiff", "tiff": "image/tiff", "avif": "image/avif",
	"mp4": "video/mp4", "m4v": "video/mp4", "mov": "video/quicktime",
	"mkv": "video/x-matroska", "webm": "video/webm", "avi": "video/x-msvideo",
}

// contentType resolves a Content-Type from the asset extension, falling back to
// the stdlib mime table. An empty result lets http.ServeContent sniff the body.
func contentType(ext string) string {
	if ct, ok := imageVideoMIME[ext]; ok {
		return ct
	}
	return mime.TypeByExtension("." + ext)
}

// serveThumb handles GET /api/wallpaper/{id}/thumb: streams the pre-generated
// thumbnail from disk. GetAsset's ThumbPath is already current-version resolved
// (SQL COALESCE), so no version lookup is needed here. 404 when the asset or its
// thumbnail is absent. Mirrors DAM's serveThumb.
func (p *Plugin) serveThumb(w http.ResponseWriter, r *http.Request) {
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
	if asset.ThumbPath == "" {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(asset.ThumbPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()

	ct := "image/jpeg"
	if strings.HasSuffix(asset.ThumbPath, ".png") {
		ct = "image/png"
	} else if strings.HasSuffix(asset.ThumbPath, ".webp") {
		ct = "image/webp"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	http.ServeContent(w, r, asset.ThumbPath, time.Time{}, f)
}

// download handles GET /api/wallpaper/{id}/download: streams the ORIGINAL bytes
// with a forced-save disposition (attachment, unlike DAM's inline /file). It is
// provider- and version-aware: the current version's provider/path is resolved
// so a versioned asset downloads its current revision, not the stale anchor.
func (p *Plugin) download(w http.ResponseWriter, r *http.Request) {
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

	providerName, storagePath, ext := p.currentFile(r.Context(), asset)
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
	defer func() { _ = f.Close() }()

	name := downloadName(asset)
	if ct := contentType(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	http.ServeContent(w, r, name, info.ModTime, f)
}

// currentFile resolves the provider, storage path, and extension to serve for
// an asset. The asset's own anchor is the default; when the asset is versioned
// (issue #58) it resolves through the current version so downloads/zip get the
// current revision's bytes, mirroring DAM's serveFile.
func (p *Plugin) currentFile(ctx context.Context, asset domain.Asset) (providerName, storagePath, ext string) {
	providerName, storagePath, ext = asset.Provider, asset.StoragePath, asset.Ext
	if asset.CurrentVersionID == "" {
		return providerName, storagePath, ext
	}
	vid, err := domain.NewVersionID(asset.CurrentVersionID)
	if err != nil {
		return providerName, storagePath, ext
	}
	v, err := p.k.Store.GetVersionByID(ctx, p.owner, vid)
	if err != nil {
		return providerName, storagePath, ext
	}
	return v.Provider, v.StoragePath, strings.ToLower(strings.TrimPrefix(path.Ext(v.StoragePath), "."))
}

// downloadName is the client-facing filename: the user-facing display name when
// set, else the indexed file name.
func downloadName(a domain.Asset) string {
	if a.DisplayName != "" {
		return a.DisplayName
	}
	return a.Name
}
