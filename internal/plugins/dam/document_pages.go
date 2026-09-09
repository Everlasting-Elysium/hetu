package dam

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// requireAsset parses the {id} path param and verifies the asset exists and
// belongs to the plugin's owner. It returns the asset on success; on failure it
// writes the HTTP error (400 bad id, 404 missing/cross-owner, 500 otherwise)
// and returns false so the caller can short-circuit. Mirrors requireBoard.
func (p *Plugin) requireAsset(w http.ResponseWriter, r *http.Request) (domain.Asset, bool) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return domain.Asset{}, false
	}
	a, err := p.k.Store.GetAsset(r.Context(), p.owner, id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpjson.WriteError(w, status, err)
		return domain.Asset{}, false
	}
	return a, true
}

// documentPageDTO is the wire form of one rendered document page. thumb_url is a
// ready-to-use API path (mounted under /api/dam) the frontend loads directly.
type documentPageDTO struct {
	PageNo   int    `json:"page_no"`
	ThumbURL string `json:"thumb_url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

// listAssetPages handles GET /assets/{id}/pages: the per-page thumbnail index of
// a multi-page document, ordered by page number. Returns 200 with [] for an
// asset that has no pages (single-page, thumbnail-less, or non-document), and
// 404 for a missing or cross-owner asset (via requireAsset).
func (p *Plugin) listAssetPages(w http.ResponseWriter, r *http.Request) {
	a, ok := p.requireAsset(w, r)
	if !ok {
		return
	}
	pages, err := p.k.Store.ListDocumentPages(r.Context(), p.owner, a.ID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]documentPageDTO, 0, len(pages))
	for _, pg := range pages {
		out = append(out, documentPageDTO{
			PageNo:   pg.PageNo,
			ThumbURL: pageThumbURL(a.ID, pg.PageNo),
			Width:    pg.Width,
			Height:   pg.Height,
		})
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// servePageThumb handles GET /assets/{id}/pages/{pageNo}/thumb: streams the
// rendered thumbnail for one page from disk, mirroring serveThumb (page thumbs
// are always JPEG). Returns 404 when the asset, page, or file is absent.
func (p *Plugin) servePageThumb(w http.ResponseWriter, r *http.Request) {
	a, ok := p.requireAsset(w, r)
	if !ok {
		return
	}
	pageNo, err := strconv.Atoi(chi.URLParam(r, "pageNo"))
	if err != nil || pageNo < 1 {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("invalid page number"))
		return
	}
	pages, err := p.k.Store.ListDocumentPages(r.Context(), p.owner, a.ID)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	thumbPath := pageThumbPath(pages, pageNo)
	if thumbPath == "" {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(thumbPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	http.ServeContent(w, r, thumbPath, time.Time{}, f)
}

// pageThumbPath returns the stored thumbnail path for pageNo, or "" when absent.
func pageThumbPath(pages []domain.DocumentPage, pageNo int) string {
	for _, pg := range pages {
		if pg.PageNo == pageNo {
			return pg.ThumbPath
		}
	}
	return ""
}

// pageThumbURL builds the API path for a page thumbnail (mounted at /api/dam).
func pageThumbURL(id domain.AssetID, pageNo int) string {
	return "/api/" + Name + "/assets/" + id.String() + "/pages/" + strconv.Itoa(pageNo) + "/thumb"
}
