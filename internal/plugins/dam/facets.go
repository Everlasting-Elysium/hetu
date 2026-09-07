package dam

import (
	"net/http"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// parseAssetFilter builds an AssetFilter from the query params shared by
// /assets, /search, and /facets: ?folder=, ?tag=, ?rating=, ?kind= (comma-
// separated), and ?status=. Kind values are whitelisted against the AssetKind
// enum (see parseKinds), so only enum values ever reach the SQL layer.
func parseAssetFilter(r *http.Request) domain.AssetFilter {
	return domain.AssetFilter{
		FolderID:  r.URL.Query().Get("folder"),
		TagID:     r.URL.Query().Get("tag"),
		MinRating: httpjson.QueryInt(r, "rating", 0),
		Kinds:     parseKinds(r.URL.Query().Get("kind")),
		Status:    r.URL.Query().Get("status"),
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
