package dam

import (
	"errors"
	"math"
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/search"
)

const (
	searchDefaultLimit = 50
	searchMaxLimit     = 200
	// defaultColorTol is the CIEDE2000 tolerance applied when the caller omits
	// tol. It is generous on purpose: a ΔE00 near 12 groups visibly similar
	// colors and avoids the over-tight matching that made Billfish frustrating.
	defaultColorTol = 12
)

// errNoSearchParam is returned when /search is called without q or color.
var errNoSearchParam = errors.New("search: provide 'q' (full-text) or 'color' (palette)")

// search handles GET /api/dam/search, dispatching by query parameter: ?color=
// runs a palette (color) search, ?q= runs a full-text (FTS5) search. When both
// are present color wins; when neither is present it is a 400.
func (p *Plugin) search(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Query().Get("color") != "":
		p.searchByColor(w, r)
	case r.URL.Query().Get("q") != "":
		p.searchByText(w, r)
	default:
		httpjson.WriteError(w, http.StatusBadRequest, errNoSearchParam)
	}
}

// searchByText handles the ?q= branch: full-text search with field qualifiers
// (name:, tag:, desc:) and boolean operators (AND, OR, NOT), ordered by FTS5
// relevance rank.
func (p *Plugin) searchByText(w http.ResponseWriter, r *http.Request) {
	ftsQuery, err := search.Parse(r.URL.Query().Get("q"))
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

// searchByColor handles the ?color= branch: it returns assets whose palette
// contains a swatch within tol (CIEDE2000) of the query color, nearest first.
func (p *Plugin) searchByColor(w http.ResponseWriter, r *http.Request) {
	rgb, err := color.ParseHex(r.URL.Query().Get("color"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	tol := httpjson.QueryInt(r, "tol", defaultColorTol)
	limit := httpjson.QueryInt(r, "limit", searchDefaultLimit)
	matches, err := p.k.Store.SearchByColor(r.Context(), p.owner, rgb.Lab(), float64(tol), limit)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toColorMatchDTOs(matches))
}

// colorMatchDTO is an asset plus which palette swatch matched and how close it
// was (CIEDE2000), letting the UI show the matched color alongside the asset.
type colorMatchDTO struct {
	assetDTO
	MatchHex string  `json:"match_hex"`
	Distance float64 `json:"color_distance"`
}

func toColorMatchDTOs(matches []domain.ColorMatch) []colorMatchDTO {
	out := make([]colorMatchDTO, 0, len(matches))
	for _, m := range matches {
		out = append(out, colorMatchDTO{
			assetDTO: toDTO(m.Asset),
			MatchHex: m.Hex,
			Distance: math.Round(m.Distance*100) / 100,
		})
	}
	return out
}
