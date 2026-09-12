package wallpaper

import (
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// list handles GET /api/wallpaper/list: the owner's live wallpapers, narrowed
// by parseWallpaperFilter (kind defaults to image,video) and ordered by ?sort=,
// paged by ?limit=/?offset=. Returns a (possibly empty) JSON array of
// wallpaperDTO.
func (p *Plugin) list(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	assets, err := p.k.Store.ListAssetsFiltered(r.Context(), p.owner, parseWallpaperFilter(r), limit, offset)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toWallpaperDTOs(assets))
}
