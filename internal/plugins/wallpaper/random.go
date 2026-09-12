package wallpaper

import (
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

const (
	// defaultRandomCount / maxRandomCount bound ?count= on /random so an
	// anonymous caller cannot pull the whole library in one shuffled request.
	defaultRandomCount = 1
	maxRandomCount     = 50
)

// random handles GET /api/wallpaper/random: up to ?count= (default 1, capped at
// maxRandomCount) wallpapers in random order, narrowed by the same facets as
// /list. When count exceeds the matching population SQLite's LIMIT simply
// returns all of them — never an error. Sort is forced to random regardless of
// any ?sort= the caller passes.
func (p *Plugin) random(w http.ResponseWriter, r *http.Request) {
	count := httpjson.QueryInt(r, "count", defaultRandomCount)
	if count < 1 {
		count = defaultRandomCount
	}
	if count > maxRandomCount {
		count = maxRandomCount
	}
	filter := parseWallpaperFilter(r)
	filter.Sort = domain.SortRandom
	assets, err := p.k.Store.ListAssetsFiltered(r.Context(), p.owner, filter, count, 0)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toWallpaperDTOs(assets))
}
