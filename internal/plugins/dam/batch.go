package dam

import (
	"fmt"
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

func (p *Plugin) batchRate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AssetIDs []string `json:"asset_ids"`
		Rating   int      `json:"rating"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ids, err := parseAssetIDs(req.AssetIDs)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if req.Rating < 0 || req.Rating > 5 {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("rating must be 0-5, got %d", req.Rating))
		return
	}
	if err := p.k.Store.BatchUpdateRating(r.Context(), p.owner, ids, req.Rating); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"updated": len(ids)})
}

func (p *Plugin) batchColor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AssetIDs []string `json:"asset_ids"`
		Color    string   `json:"color"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ids, err := parseAssetIDs(req.AssetIDs)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.BatchUpdateColor(r.Context(), p.owner, ids, req.Color); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"updated": len(ids)})
}

func (p *Plugin) batchMove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AssetIDs []string `json:"asset_ids"`
		FolderID string   `json:"folder_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ids, err := parseAssetIDs(req.AssetIDs)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.BatchMoveToFolder(r.Context(), p.owner, ids, req.FolderID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"moved": len(ids)})
}

func (p *Plugin) batchTrash(w http.ResponseWriter, r *http.Request) {
	ids, ok := p.decodeAssetIDs(w, r)
	if !ok {
		return
	}
	if err := p.k.Store.BatchTrashAssets(r.Context(), p.owner, ids); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"trashed": len(ids)})
}

func (p *Plugin) batchRestore(w http.ResponseWriter, r *http.Request) {
	ids, ok := p.decodeAssetIDs(w, r)
	if !ok {
		return
	}
	if err := p.k.Store.BatchRestoreAssets(r.Context(), p.owner, ids); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"restored": len(ids)})
}

func (p *Plugin) listTrash(w http.ResponseWriter, r *http.Request) {
	limit := httpjson.QueryInt(r, "limit", 50)
	offset := httpjson.QueryInt(r, "offset", 0)
	assets, err := p.k.Store.ListTrashedAssets(r.Context(), p.owner, limit, offset)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDTOs(assets, nil))
}

func (p *Plugin) emptyTrash(w http.ResponseWriter, r *http.Request) {
	retentionDays := httpjson.QueryInt(r, "retention_days", 0)
	if err := p.k.Store.PurgeTrash(r.Context(), p.owner, retentionDays); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"purged": true})
}

// decodeAssetIDs decodes a body of the shape {"asset_ids": [...]}, writing the
// appropriate error response and returning ok=false on failure.
func (p *Plugin) decodeAssetIDs(w http.ResponseWriter, r *http.Request) ([]domain.AssetID, bool) {
	var req struct {
		AssetIDs []string `json:"asset_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return nil, false
	}
	ids, err := parseAssetIDs(req.AssetIDs)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return nil, false
	}
	return ids, true
}
