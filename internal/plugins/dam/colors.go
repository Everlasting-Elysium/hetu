package dam

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

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

// addAssetColor appends a hand-picked swatch to an asset's palette:
// POST /api/dam/assets/{id}/colors  {"hex":"#rrggbb"} -> updated [{hex,weight}].
// The edit flags the palette user-curated so a re-scan will not overwrite it (#62).
func (p *Plugin) addAssetColor(w http.ResponseWriter, r *http.Request) {
	id, ok := p.paletteAsset(w, r)
	if !ok {
		return
	}
	rgb, ok := decodeHexBody(w, r)
	if !ok {
		return
	}
	swatches, err := p.k.Store.AddAssetColor(r.Context(), p.owner, id, rgb)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, swatches)
}

// updateAssetColor repoints one swatch at a new color:
// PUT /api/dam/assets/{id}/colors/{ord}  {"hex":"#rrggbb"} -> updated palette.
// A missing ord answers 404.
func (p *Plugin) updateAssetColor(w http.ResponseWriter, r *http.Request) {
	id, ok := p.paletteAsset(w, r)
	if !ok {
		return
	}
	ord, ok := parseColorOrd(w, r)
	if !ok {
		return
	}
	rgb, ok := decodeHexBody(w, r)
	if !ok {
		return
	}
	swatches, err := p.k.Store.UpdateAssetColor(r.Context(), p.owner, id, ord, rgb)
	if err != nil {
		httpjson.WriteError(w, colorEditStatus(err), err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, swatches)
}

// deleteAssetColor removes one swatch and renumbers the survivors to a
// contiguous 0..N-1: DELETE /api/dam/assets/{id}/colors/{ord} -> updated palette.
// A missing ord answers 404.
func (p *Plugin) deleteAssetColor(w http.ResponseWriter, r *http.Request) {
	id, ok := p.paletteAsset(w, r)
	if !ok {
		return
	}
	ord, ok := parseColorOrd(w, r)
	if !ok {
		return
	}
	swatches, err := p.k.Store.DeleteAssetColor(r.Context(), p.owner, id, ord)
	if err != nil {
		httpjson.WriteError(w, colorEditStatus(err), err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, swatches)
}

// paletteAsset loads the asset addressed by {id} for a palette-edit request,
// enforcing the same guards the read path uses: a malformed id is 400, a missing
// or other-owner asset is 404 (GetAsset is owner-scoped), and a kind that has no
// color palette (audio, #88) is rejected 400 rather than 500. It returns
// ok=false after writing the response when the request must not proceed.
func (p *Plugin) paletteAsset(w http.ResponseWriter, r *http.Request) (domain.AssetID, bool) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return id, false
	}
	asset, err := p.k.Store.GetAsset(r.Context(), p.owner, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return id, false
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return id, false
	}
	if !asset.Kind.SupportsColorPalette() {
		httpjson.WriteError(w, http.StatusBadRequest,
			fmt.Errorf("asset kind %q has no color palette", asset.Kind))
		return id, false
	}
	return id, true
}

// decodeHexBody decodes a {"hex":"#rrggbb"} body into an RGB, writing a 400 and
// returning ok=false on a malformed body or an unparseable hex color.
func decodeHexBody(w http.ResponseWriter, r *http.Request) (color.RGB, bool) {
	var req struct {
		Hex string `json:"hex"`
	}
	if !decodeJSON(w, r, &req) {
		return color.RGB{}, false
	}
	rgb, err := color.ParseHex(req.Hex)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return color.RGB{}, false
	}
	return rgb, true
}

// parseColorOrd parses the {ord} path param (a non-negative swatch index),
// writing a 400 and returning ok=false when it is missing or malformed.
func parseColorOrd(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := chi.URLParam(r, "ord")
	ord, err := strconv.Atoi(raw)
	if err != nil || ord < 0 {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid ord %q", raw))
		return 0, false
	}
	return ord, true
}

// colorEditStatus maps a palette-edit store error to its HTTP status: a missing
// swatch ord is 404, anything else 500.
func colorEditStatus(err error) int {
	if errors.Is(err, domain.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
