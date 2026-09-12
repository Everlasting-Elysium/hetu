package wallpaper

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// collectionDTO is the public wire form of a wallpaper collection. CoverURL is
// the /thumb endpoint of the collection's effective cover asset when that asset
// exists and has a thumbnail, otherwise omitted — the raw cover asset id is
// never exposed.
type collectionDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	CoverURL string `json:"cover_url,omitempty"`
}

// listCollections handles GET /api/wallpaper/collections: every collection with
// its resolved cover thumbnail URL (when resolvable). The store already folds
// the effective cover (explicit override, else lowest-ord member) into
// Collection.Cover; this handler only turns that asset id into a thumb URL.
func (p *Plugin) listCollections(w http.ResponseWriter, r *http.Request) {
	cols, err := p.k.Store.ListCollections(r.Context(), p.owner)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]collectionDTO, 0, len(cols))
	for _, c := range cols {
		dto := collectionDTO{ID: c.ID.String(), Name: c.Name}
		if url, ok := p.coverURL(r.Context(), c.Cover); ok {
			dto.CoverURL = url
		}
		out = append(out, dto)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// coverURL resolves a cover asset id into its wallpaper /thumb URL. ok is false
// (cover omitted, never an error) when the id is empty, unparseable, missing, or
// the asset has no thumbnail — a broken cover must not fail the whole listing.
func (p *Plugin) coverURL(ctx context.Context, cover string) (string, bool) {
	if cover == "" {
		return "", false
	}
	aid, err := domain.NewAssetID(cover)
	if err != nil {
		return "", false
	}
	asset, err := p.k.Store.GetAsset(ctx, p.owner, aid)
	if err != nil || asset.ThumbPath == "" {
		return "", false
	}
	return fmt.Sprintf("/api/wallpaper/%s/thumb", aid.String()), true
}

// getCollectionAssets handles GET /api/wallpaper/collections/{id}: the
// collection's members as full wallpaperDTOs, in manual (ord) order, paged. It
// does NOT apply parseWallpaperFilter — a collection is an already-curated set,
// so the issue orders it by ord without layering kind/shape facets on top. 404
// when the collection does not exist or belong to the owner.
func (p *Plugin) getCollectionAssets(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewCollectionID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	limit, offset := pageParams(r)
	assets, err := p.k.Store.ListCollectionAssets(r.Context(), p.owner, id, limit, offset)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toWallpaperDTOs(assets))
}
