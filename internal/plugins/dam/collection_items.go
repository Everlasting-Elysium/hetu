package dam

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// requireCollection verifies the collection exists and belongs to the plugin's
// owner. Returns the parsed CollectionID on success; writes the HTTP error and
// returns false on failure so the caller can short-circuit. Mirrors requireBoard.
func (p *Plugin) requireCollection(w http.ResponseWriter, r *http.Request) (domain.CollectionID, bool) {
	cid, err := domain.NewCollectionID(r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return cid, false
	}
	if _, err := p.k.Store.GetCollection(r.Context(), p.owner, cid); err != nil {
		httpjson.WriteError(w, collectionErrStatus(err), err)
		return cid, false
	}
	return cid, true
}

// collectionItemDTO is the wire form of a collection membership, enriched with
// its asset's kind/name/thumb (resolved by the store in one JOIN) so the
// frontend can render a member without a second asset query.
type collectionItemDTO struct {
	AssetID    string `json:"asset_id"`
	Ord        int    `json:"ord"`
	AssetKind  string `json:"asset_kind"`
	AssetName  string `json:"asset_name"`
	AssetThumb string `json:"asset_thumb"`
}

func toCollectionItemDTO(it domain.CollectionItem) collectionItemDTO {
	return collectionItemDTO{
		AssetID:    it.AssetID.String(),
		Ord:        it.Ord,
		AssetKind:  it.AssetKind,
		AssetName:  it.AssetName,
		AssetThumb: it.AssetThumb,
	}
}

func (p *Plugin) listCollectionItems(w http.ResponseWriter, r *http.Request) {
	cid, ok := p.requireCollection(w, r)
	if !ok {
		return
	}
	items, err := p.k.Store.ListCollectionItems(r.Context(), p.owner, cid)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]collectionItemDTO, 0, len(items))
	for _, it := range items {
		out = append(out, toCollectionItemDTO(it))
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (p *Plugin) addCollectionItem(w http.ResponseWriter, r *http.Request) {
	cid, ok := p.requireCollection(w, r)
	if !ok {
		return
	}
	var req struct {
		AssetID string `json:"asset_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	aid, err := domain.NewAssetID(req.AssetID)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid asset_id: %w", err))
		return
	}
	// A not-found asset covers two cases: no such asset, and an asset owned by
	// someone else (AddCollectionItem scopes the lookup to p.owner) — both 404,
	// neither leaks which case it was.
	if err := p.k.Store.AddCollectionItem(r.Context(), p.owner, cid, aid); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpjson.WriteError(w, status, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, map[string]bool{"added": true})
}

func (p *Plugin) removeCollectionItem(w http.ResponseWriter, r *http.Request) {
	cid, ok := p.requireCollection(w, r)
	if !ok {
		return
	}
	aid, err := domain.NewAssetID(r.PathValue("assetId"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.RemoveCollectionItem(r.Context(), p.owner, cid, aid); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// reorderCollectionItems rewrites the whole ord sequence from the request's
// asset_ids. The store rejects a set that does not match the current members
// exactly with ErrCollectionItemsMismatch, which maps to 400.
func (p *Plugin) reorderCollectionItems(w http.ResponseWriter, r *http.Request) {
	cid, ok := p.requireCollection(w, r)
	if !ok {
		return
	}
	var req struct {
		AssetIDs []string `json:"asset_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ids, err := parseAssetIDs(req.AssetIDs)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.ReorderCollectionItems(r.Context(), p.owner, cid, ids); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrCollectionItemsMismatch) {
			status = http.StatusBadRequest
		}
		httpjson.WriteError(w, status, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"reordered": len(ids)})
}
