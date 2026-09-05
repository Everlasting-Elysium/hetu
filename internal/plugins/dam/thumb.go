package dam

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// serveThumb handles GET /assets/{id}/thumb: streams the pre-generated
// thumbnail from disk. Returns 404 when the asset does not exist or has no
// thumbnail yet.
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
	defer f.Close()

	// Infer content type from extension; default to JPEG (the thumbnail
	// generator writes JPEG).
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
