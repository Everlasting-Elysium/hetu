package dam

import (
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// parseAssetFilter builds an AssetFilter from the query params shared by
// /assets, /search, and /facets: ?folder=, ?tag=, ?rating=, ?kind= (comma-
// separated), ?status=, the issue #101 range/shape params ?minSize=/?maxSize=
// (bytes), ?minWidth=/?maxWidth=/?minHeight=/?maxHeight= (pixels), ?shape=
// (comma-separated landscape/portrait/square), and the issue #53 params
// ?minDuration=/?maxDuration= (seconds, float) and ?createdAfter=/?createdBefore=/
// ?indexedAfter=/?indexedBefore= (unix seconds). Kind and shape values are
// whitelisted against their enums (domain.ParseKinds/ParseShapes) and every
// range is clamped by httpjson.NormalizeRange/NormalizeRangeFloat, so only sane,
// enum-checked values reach the SQL layer. Those helpers are shared with the
// wallpaper plugin (issue #114), so the whitelist/clamp logic lives once in
// domain/httpjson rather than being copied per plugin.
func parseAssetFilter(r *http.Request) domain.AssetFilter {
	minSize, maxSize := httpjson.NormalizeRange(httpjson.QueryInt64(r, "minSize", 0), httpjson.QueryInt64(r, "maxSize", 0))
	minW, maxW := httpjson.NormalizeRange(int64(httpjson.QueryInt(r, "minWidth", 0)), int64(httpjson.QueryInt(r, "maxWidth", 0)))
	minH, maxH := httpjson.NormalizeRange(int64(httpjson.QueryInt(r, "minHeight", 0)), int64(httpjson.QueryInt(r, "maxHeight", 0)))
	minDur, maxDur := httpjson.NormalizeRangeFloat(httpjson.QueryFloat64(r, "minDuration", 0), httpjson.QueryFloat64(r, "maxDuration", 0))
	createdAfter, createdBefore := httpjson.NormalizeRange(httpjson.QueryInt64(r, "createdAfter", 0), httpjson.QueryInt64(r, "createdBefore", 0))
	indexedAfter, indexedBefore := httpjson.NormalizeRange(httpjson.QueryInt64(r, "indexedAfter", 0), httpjson.QueryInt64(r, "indexedBefore", 0))
	return domain.AssetFilter{
		FolderID:      r.URL.Query().Get("folder"),
		TagID:         r.URL.Query().Get("tag"),
		MinRating:     httpjson.QueryInt(r, "rating", 0),
		Kinds:         domain.ParseKinds(r.URL.Query().Get("kind")),
		Status:        r.URL.Query().Get("status"),
		MinSize:       minSize,
		MaxSize:       maxSize,
		MinWidth:      int(minW),
		MaxWidth:      int(maxW),
		MinHeight:     int(minH),
		MaxHeight:     int(maxH),
		Shapes:        domain.ParseShapes(r.URL.Query().Get("shape")),
		MinDuration:   minDur,
		MaxDuration:   maxDur,
		CreatedAfter:  createdAfter,
		CreatedBefore: createdBefore,
		IndexedAfter:  indexedAfter,
		IndexedBefore: indexedBefore,
	}
}

// kindCount is one row of the format facet: a kind and how many live assets have
// it in the current folder/tag/rating context.
type kindCount struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// facetsResponse drives the sidebar/board format facet. Kinds lists every
// AssetKind in display order (including zero counts) so the UI renders a stable
// facet and can hide empty formats itself.
type facetsResponse struct {
	Kinds []kindCount `json:"kinds"`
}

// facets returns the per-kind counts for the format facet, narrowed by the same
// folder/tag/rating context as /assets. The active kind selection is ignored so
// every format keeps a count and stays selectable while multi-selecting.
func (p *Plugin) facets(w http.ResponseWriter, r *http.Request) {
	counts, err := p.k.Store.KindCounts(r.Context(), p.owner, parseAssetFilter(r))
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	kinds := make([]kindCount, 0, len(domain.AllKinds))
	for _, k := range domain.AllKinds {
		kinds = append(kinds, kindCount{Kind: string(k), Count: counts[k]})
	}
	httpjson.WriteJSON(w, http.StatusOK, facetsResponse{Kinds: kinds})
}
