package dam

import (
	"net/http"
	"strings"

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
// whitelisted against their enums (see parseKinds/parseShapes) and every range
// is clamped by normalizeRange/normalizeRangeFloat, so only sane, enum-checked
// values reach the SQL layer.
func parseAssetFilter(r *http.Request) domain.AssetFilter {
	minSize, maxSize := normalizeRange(httpjson.QueryInt64(r, "minSize", 0), httpjson.QueryInt64(r, "maxSize", 0))
	minW, maxW := normalizeRange(int64(httpjson.QueryInt(r, "minWidth", 0)), int64(httpjson.QueryInt(r, "maxWidth", 0)))
	minH, maxH := normalizeRange(int64(httpjson.QueryInt(r, "minHeight", 0)), int64(httpjson.QueryInt(r, "maxHeight", 0)))
	minDur, maxDur := normalizeRangeFloat(httpjson.QueryFloat64(r, "minDuration", 0), httpjson.QueryFloat64(r, "maxDuration", 0))
	createdAfter, createdBefore := normalizeRange(httpjson.QueryInt64(r, "createdAfter", 0), httpjson.QueryInt64(r, "createdBefore", 0))
	indexedAfter, indexedBefore := normalizeRange(httpjson.QueryInt64(r, "indexedAfter", 0), httpjson.QueryInt64(r, "indexedBefore", 0))
	return domain.AssetFilter{
		FolderID:      r.URL.Query().Get("folder"),
		TagID:         r.URL.Query().Get("tag"),
		MinRating:     httpjson.QueryInt(r, "rating", 0),
		Kinds:         parseKinds(r.URL.Query().Get("kind")),
		Status:        r.URL.Query().Get("status"),
		MinSize:       minSize,
		MaxSize:       maxSize,
		MinWidth:      int(minW),
		MaxWidth:      int(maxW),
		MinHeight:     int(minH),
		MaxHeight:     int(maxH),
		Shapes:        parseShapes(r.URL.Query().Get("shape")),
		MinDuration:   minDur,
		MaxDuration:   maxDur,
		CreatedAfter:  createdAfter,
		CreatedBefore: createdBefore,
		IndexedAfter:  indexedAfter,
		IndexedBefore: indexedBefore,
	}
}

// parseKinds splits a comma-separated ?kind= value into known AssetKinds,
// silently dropping blanks and any token outside the enum. This whitelist is the
// injection guard: an unknown string never reaches the a.kind IN (...) clause.
func parseKinds(raw string) []domain.AssetKind {
	if raw == "" {
		return nil
	}
	var kinds []domain.AssetKind
	for _, tok := range strings.Split(raw, ",") {
		if tok = strings.TrimSpace(tok); domain.ValidKind(tok) {
			kinds = append(kinds, domain.AssetKind(tok))
		}
	}
	return kinds
}

// normalizeRange clamps a min/max pair to hetu's range-filter contract:
// negative values collapse to 0 (unbounded on that side), and an inverted
// range (max>0 AND max<min) drops max back to 0 (unbounded) rather than
// swapping the two — a confused range widens instead of silently
// reinterpreting the caller's numbers in swapped roles (issue #101 design
// decision 3; the issue explicitly permits either "ignore max" or "swap" —
// this picks "ignore max").
func normalizeRange(min, max int64) (int64, int64) {
	if min < 0 {
		min = 0
	}
	if max < 0 {
		max = 0
	}
	if max > 0 && max < min {
		max = 0
	}
	return min, max
}

// normalizeRangeFloat is normalizeRange for float64 ranges (the duration facet,
// seconds — which may be fractional, so it cannot reuse the int64 version). Same
// contract: negative -> 0 (unbounded), inverted max<min -> drop max to unbounded
// rather than swapping (issue #53).
func normalizeRangeFloat(min, max float64) (float64, float64) {
	if min < 0 {
		min = 0
	}
	if max < 0 {
		max = 0
	}
	if max > 0 && max < min {
		max = 0
	}
	return min, max
}

// parseShapes splits a comma-separated ?shape= value into known AssetShapes,
// mirroring parseKinds exactly — the whitelist guard before any value reaches
// the SQL layer.
func parseShapes(raw string) []domain.AssetShape {
	if raw == "" {
		return nil
	}
	var shapes []domain.AssetShape
	for _, tok := range strings.Split(raw, ",") {
		if tok = strings.TrimSpace(tok); domain.ValidShape(tok) {
			shapes = append(shapes, domain.AssetShape(tok))
		}
	}
	return shapes
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
