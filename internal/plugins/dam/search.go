package dam

import (
	"errors"
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/search"
)

const (
	searchDefaultLimit = 50
	searchMaxLimit     = 200
)

// searchAssets handles GET /api/dam/search?q=<query>&limit=&offset=.
// The q parameter accepts field qualifiers (name:, tag:, desc:) and boolean
// operators (AND, OR, NOT); results are ordered by FTS5 relevance rank.
func (p *Plugin) searchAssets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	ftsQuery, err := search.Parse(q)
	if err != nil {
		// The only parse error is an empty/blank query -> 400.
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}

	limit := httpjson.QueryInt(r, "limit", searchDefaultLimit)
	if limit <= 0 || limit > searchMaxLimit {
		limit = searchDefaultLimit
	}
	offset := httpjson.QueryInt(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}

	assets, err := p.k.Store.SearchAssets(r.Context(), p.owner, ftsQuery, limit, offset)
	if err != nil {
		// A malformed FTS5 query is client error; anything else is a server fault.
		if errors.Is(err, domain.ErrInvalidQuery) {
			httpjson.WriteError(w, http.StatusBadRequest, err)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDTOs(assets))
}
