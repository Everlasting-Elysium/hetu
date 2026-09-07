package dam

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// assetColors returns an asset's extracted palette, dominant color first:
// GET /api/dam/assets/{id}/colors -> [{"hex":"#rrggbb","weight":0.42}, ...].
// An asset with no extracted palette yields an empty array (200), not 404.
// The response shape matches color.Swatch.MarshalJSON (weight rounded to 4 dp).
func (p *Plugin) assetColors(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	asset, err := p.k.Store.GetAsset(r.Context(), p.owner, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	// Audio carries no meaningful color (its thumbnail is a waveform), so it
	// never offers color swatches — answer [] even if stale rows linger (#88).
	if !asset.Kind.SupportsColorPalette() {
		httpjson.WriteJSON(w, http.StatusOK, []color.Swatch{})
		return
	}
	swatches, err := p.k.Store.ListAssetColors(r.Context(), p.owner, id)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, swatches)
}
