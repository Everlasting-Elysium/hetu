package wallpaper

import (
	"net/http"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// dailyLocation is the fixed timezone the daily rollover is anchored to
// (Asia/Shanghai). If the zoneinfo database is unavailable (minimal containers)
// it falls back to a fixed UTC+8 offset rather than failing the request — the
// only cost is no DST handling, which China does not observe anyway.
func dailyLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*3600)
}

// daily handles GET /api/wallpaper/daily: one deterministic "wallpaper of the
// day". The pick is count + a date-seeded offset (NOT RANDOM()), so every
// request on the same calendar day (Asia/Shanghai) returns the same asset and
// it rotates at local midnight. Sort is forced to latest so the offset indexes
// a stable order. 404 when nothing matches the filter.
func (p *Plugin) daily(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	filter := parseWallpaperFilter(r)
	filter.Sort = domain.SortLatest

	count, err := p.k.Store.CountAssetsFiltered(ctx, p.owner, filter)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if count == 0 {
		http.NotFound(w, r)
		return
	}

	// Seed = the plain decimal date (e.g. 20260912): constant within a day,
	// changes across days, reproducible — no unstable hash. idx stays in range
	// via % count.
	now := p.now().In(dailyLocation())
	seed := now.Year()*10000 + int(now.Month())*100 + now.Day()
	idx := seed % count

	assets, err := p.k.Store.ListAssetsFiltered(ctx, p.owner, filter, 1, idx)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if len(assets) == 0 {
		http.NotFound(w, r)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toWallpaperDTO(assets[0]))
}
