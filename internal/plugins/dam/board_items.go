package dam

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// requireBoard verifies the board exists and belongs to the plugin's owner.
// Returns the parsed BoardID on success; writes the HTTP error and returns
// false on failure so the caller can short-circuit.
func (p *Plugin) requireBoard(w http.ResponseWriter, r *http.Request) (domain.BoardID, bool) {
	bid, err := domain.NewBoardID(r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return bid, false
	}
	if _, err := p.k.Store.GetBoard(r.Context(), p.owner, bid); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpjson.WriteError(w, status, err)
		return bid, false
	}
	return bid, true
}

// boardItemDTO is the wire form of a placed board item — either an asset
// placement or a text note (see domain.BoardItemKind). Text, FrameMS, and View
// are omitted from the response when unused. AssetKind/AssetName/AssetThumb are
// server-resolved from the asset table (issue #86) so the frontend can identify
// a placed video/model/image without querying the asset panel; they are only
// populated for asset items and stay empty for notes.
type boardItemDTO struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	AssetID    string  `json:"asset_id"`
	Text       string  `json:"text,omitempty"`
	FrameMS    *int64  `json:"frame_ms,omitempty"`
	View       string  `json:"view,omitempty"`
	AssetKind  string  `json:"asset_kind,omitempty"`
	AssetName  string  `json:"asset_name,omitempty"`
	AssetThumb string  `json:"asset_thumb,omitempty"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	W          float64 `json:"w"`
	H          float64 `json:"h"`
	Rotation   float64 `json:"rotation"`
	Z          int     `json:"z"`
}

func toBoardItemDTO(it domain.BoardItem) boardItemDTO {
	return boardItemDTO{
		ID: it.ID.String(), Kind: string(it.Kind), AssetID: it.AssetID.String(),
		Text: it.Text, FrameMS: it.FrameMS, View: it.View,
		X: it.X, Y: it.Y, W: it.W, H: it.H,
		Rotation: it.Rotation, Z: it.Z,
	}
}

// enrichItemAssets fills asset_kind/asset_name/asset_thumb on every asset item
// so the frontend can identify a placed video/model/image without querying the
// asset panel (issue #86). Note items are left untouched. Each distinct asset_id
// is resolved at most once; an asset that no longer exists (deleted) leaves that
// item's asset fields empty rather than failing the whole request.
func (p *Plugin) enrichItemAssets(ctx context.Context, dtos []boardItemDTO) []boardItemDTO {
	type assetMeta struct{ kind, name, thumb string }
	resolved := make(map[string]assetMeta)
	for i := range dtos {
		item := &dtos[i]
		if item.Kind != string(domain.BoardItemAsset) || item.AssetID == "" {
			continue
		}
		meta, done := resolved[item.AssetID]
		if !done {
			if aid, err := domain.NewAssetID(item.AssetID); err == nil {
				if a, err := p.k.Store.GetAsset(ctx, p.owner, aid); err == nil {
					name := a.DisplayName
					if name == "" {
						name = a.Name
					}
					meta = assetMeta{kind: string(a.Kind), name: name, thumb: a.ThumbPath}
				} else if !errors.Is(err, domain.ErrNotFound) {
					p.k.Log.WarnContext(ctx, "enrich board item: get asset",
						slog.String("asset", item.AssetID), slog.Any("err", err))
				}
			}
			resolved[item.AssetID] = meta
		}
		item.AssetKind, item.AssetName, item.AssetThumb = meta.kind, meta.name, meta.thumb
	}
	return dtos
}

func (p *Plugin) addBoardItem(w http.ResponseWriter, r *http.Request) {
	bid, ok := p.requireBoard(w, r)
	if !ok {
		return
	}
	var err error
	var req struct {
		Kind     string  `json:"kind"`
		AssetID  string  `json:"asset_id"`
		Text     string  `json:"text"`
		FrameMS  *int64  `json:"frame_ms"`
		View     string  `json:"view"`
		X        float64 `json:"x"`
		Y        float64 `json:"y"`
		W        float64 `json:"w"`
		H        float64 `json:"h"`
		Rotation float64 `json:"rotation"`
		Z        int     `json:"z"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	kind := domain.BoardItemKind(req.Kind)
	if kind == "" {
		kind = domain.BoardItemAsset
	}
	var aid domain.AssetID // note items carry no asset_id
	if req.AssetID != "" {
		aid, err = domain.NewAssetID(req.AssetID)
		if err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid asset_id: %w", err))
			return
		}
	}
	id, err := newID()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	iid, _ := domain.NewBoardItemID(id)
	now := time.Now().UTC().Truncate(time.Second)
	item := domain.BoardItem{
		ID: iid, BoardID: bid, Kind: kind, AssetID: aid,
		Text: req.Text, FrameMS: req.FrameMS, View: req.View,
		X: req.X, Y: req.Y, W: req.W, H: req.H,
		Rotation: req.Rotation, Z: req.Z, CreatedAt: now,
	}
	if err := domain.ValidateBoardItem(item); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	got, err := p.k.Store.AddBoardItem(r.Context(), item)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	// Enrich so a freshly dropped video/model is immediately re-editable without
	// the frontend re-querying the asset panel (issue #86). Notes pass through.
	enriched := p.enrichItemAssets(r.Context(), []boardItemDTO{toBoardItemDTO(got)})
	httpjson.WriteJSON(w, http.StatusCreated, enriched[0])
}

func (p *Plugin) updateBoardItems(w http.ResponseWriter, r *http.Request) {
	bid, ok := p.requireBoard(w, r)
	if !ok {
		return
	}
	var req struct {
		Items []boardItemDTO `json:"items"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	updates := make([]domain.BoardItem, 0, len(req.Items))
	for _, it := range req.Items {
		iid, err := domain.NewBoardItemID(it.ID)
		if err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid item id %q: %w", it.ID, err))
			return
		}
		kind := domain.BoardItemKind(it.Kind)
		if kind == "" {
			kind = domain.BoardItemAsset
		}
		var aid domain.AssetID // note items carry no asset_id
		if it.AssetID != "" {
			aid, err = domain.NewAssetID(it.AssetID)
			if err != nil {
				httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid asset_id %q: %w", it.AssetID, err))
				return
			}
		}
		item := domain.BoardItem{
			ID: iid, BoardID: bid, Kind: kind, AssetID: aid,
			Text: it.Text, FrameMS: it.FrameMS, View: it.View,
			X: it.X, Y: it.Y, W: it.W, H: it.H,
			Rotation: it.Rotation, Z: it.Z,
		}
		if err := domain.ValidateBoardItem(item); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("item %q: %w", it.ID, err))
			return
		}
		updates = append(updates, item)
	}
	if err := p.k.Store.BatchUpdateBoardItems(r.Context(), bid, updates); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"updated": len(updates)})
}

func (p *Plugin) deleteBoardItem(w http.ResponseWriter, r *http.Request) {
	bid, ok := p.requireBoard(w, r)
	if !ok {
		return
	}
	iid, err := domain.NewBoardItemID(r.PathValue("itemId"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.DeleteBoardItem(r.Context(), bid, iid); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
