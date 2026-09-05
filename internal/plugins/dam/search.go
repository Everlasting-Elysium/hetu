package dam

import (
	"math"
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// defaultColorTol is the CIEDE2000 tolerance applied when the caller omits tol.
// It is generous on purpose: a ΔE00 near 12 groups visibly similar colors and
// avoids the over-tight matching that made Billfish's color search frustrating.
const defaultColorTol = 12

// searchByColor handles GET /api/dam/search?color=<hex>&tol=<n>&limit=<n>: it
// returns assets whose palette contains a swatch within tol (CIEDE2000) of the
// query color, nearest first.
func (p *Plugin) searchByColor(w http.ResponseWriter, r *http.Request) {
	rgb, err := color.ParseHex(r.URL.Query().Get("color"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	tol := httpjson.QueryInt(r, "tol", defaultColorTol)
	limit := httpjson.QueryInt(r, "limit", 50)
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
