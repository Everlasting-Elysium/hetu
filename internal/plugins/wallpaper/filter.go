package wallpaper

import (
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

const (
	// defaultLimit / maxLimit bound the ?limit= page size on /list and
	// /collections/{id}. DAM's /assets defaults to 50 with no ceiling; the
	// public wallpaper endpoints use a smaller default and a hard cap so an
	// anonymous caller cannot request an unbounded page (issue #114).
	defaultLimit = 30
	maxLimit     = 100
)

// parseWallpaperFilter builds an AssetFilter from the wallpaper query params:
// ?kind= (comma-separated, DEFAULTS to image,video when absent/all-invalid —
// unlike DAM where empty means "any format"), ?shape=, ?minWidth=/?maxWidth=/
// ?minHeight=/?maxHeight=, ?rating= (min stars), ?collection= (a single
// collection's members), and ?sort= (latest/rating/random, invalid falls back
// to the default order). It reuses the shared domain/httpjson whitelist+clamp
// helpers rather than copying DAM's parser.
func parseWallpaperFilter(r *http.Request) domain.AssetFilter {
	minW, maxW := httpjson.NormalizeRange(int64(httpjson.QueryInt(r, "minWidth", 0)), int64(httpjson.QueryInt(r, "maxWidth", 0)))
	minH, maxH := httpjson.NormalizeRange(int64(httpjson.QueryInt(r, "minHeight", 0)), int64(httpjson.QueryInt(r, "maxHeight", 0)))
	kinds := domain.ParseKinds(r.URL.Query().Get("kind"))
	if len(kinds) == 0 {
		// Wallpapers are images and videos by default; a blank or fully-invalid
		// ?kind= must never widen to every format (fonts, 3D, documents...).
		kinds = []domain.AssetKind{domain.KindImage, domain.KindVideo}
	}
	return domain.AssetFilter{
		Kinds:        kinds,
		Shapes:       domain.ParseShapes(r.URL.Query().Get("shape")),
		MinWidth:     int(minW),
		MaxWidth:     int(maxW),
		MinHeight:    int(minH),
		MaxHeight:    int(maxH),
		MinRating:    httpjson.QueryInt(r, "rating", 0),
		CollectionID: r.URL.Query().Get("collection"),
		Sort:         parseSort(r.URL.Query().Get("sort")),
	}
}

// parseSort whitelists ?sort= against domain.ValidSort; an empty or unknown
// value returns the zero AssetSort, which the store maps to the default
// (indexed_at DESC) — an invalid sort is ignored, never a 400.
func parseSort(raw string) domain.AssetSort {
	if domain.ValidSort(raw) {
		return domain.AssetSort(raw)
	}
	return ""
}

// pageParams reads ?limit=/?offset= with the wallpaper defaults and caps: limit
// clamps into [1, maxLimit] (a non-positive or over-cap value snaps to the
// default/cap), offset floors at 0. Shared by /list and /collections/{id}.
func pageParams(r *http.Request) (limit, offset int) {
	limit = httpjson.QueryInt(r, "limit", defaultLimit)
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset = httpjson.QueryInt(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
