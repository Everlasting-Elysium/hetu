package dam

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// Deterministic default layout for assets sent to a board from the list view
// (issue #76). New items tile in a fixed-width grid placed below any existing
// content so nothing stacks at (0,0); precise arranging is left to the canvas.
const (
	boardItemDefaultSize = 200.0 // matches the board_items.w/h schema default
	boardBatchCols       = 5     // grid columns for one batch drop
	boardBatchGap        = 24.0  // gap between cells and above existing items
)

// batchAddBoardItems adds many assets to a board in one call — the "send to
// board" action from the asset list. Body: {"asset_ids": ["..."]}. It validates
// the board is owned and every asset exists and is owned, skips assets already
// on the board (board_items has no unique constraint, so de-dup is explicit),
// lays the rest out with a deterministic grid, and returns {"added": <n>} where
// n counts only the items actually inserted (duplicates excluded).
func (p *Plugin) batchAddBoardItems(w http.ResponseWriter, r *http.Request) {
	bid, err := domain.NewBoardID(r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
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
	if _, err := p.k.Store.GetBoard(r.Context(), p.owner, bid); err != nil {
		httpjson.WriteError(w, notFoundOr500(err), err)
		return
	}
	existing, err := p.k.Store.ListBoardItems(r.Context(), bid)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	items, err := p.planBatchAddItems(r.Context(), bid, ids, existing)
	if err != nil {
		httpjson.WriteError(w, notFoundOr500(err), err)
		return
	}
	if err := p.k.Store.BatchAddBoardItems(r.Context(), bid, items); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"added": len(items)})
}

// planBatchAddItems validates and lays out the assets to insert. Each asset must
// resolve to one owned by p.owner (else the error wraps domain.ErrNotFound);
// assets already on the board — or repeated within the request — are skipped.
// Survivors tile in a boardBatchCols-wide grid starting below the lowest
// existing item, each sized to the schema default and z-ordered above existing
// content so a freshly-sent batch lands on top.
func (p *Plugin) planBatchAddItems(ctx context.Context, bid domain.BoardID, ids []domain.AssetID, existing []domain.BoardItem) ([]domain.BoardItem, error) {
	onBoard := make(map[string]bool, len(existing))
	startY, baseZ := 0.0, 0
	for _, it := range existing {
		onBoard[it.AssetID.String()] = true
		if bottom := it.Y + it.H; bottom > startY {
			startY = bottom
		}
		if it.Z > baseZ {
			baseZ = it.Z
		}
	}
	if startY > 0 {
		startY += boardBatchGap
	}
	baseZ++
	stride := boardItemDefaultSize + boardBatchGap
	now := time.Now().UTC().Truncate(time.Second)

	items := make([]domain.BoardItem, 0, len(ids))
	for _, aid := range ids {
		key := aid.String()
		if onBoard[key] {
			continue // already on the board, or a duplicate earlier in this request
		}
		onBoard[key] = true
		if _, err := p.k.Store.GetAsset(ctx, p.owner, aid); err != nil {
			return nil, fmt.Errorf("asset %s: %w", aid, err)
		}
		id, err := newID()
		if err != nil {
			return nil, err
		}
		iid, _ := domain.NewBoardItemID(id)
		n := len(items)
		items = append(items, domain.BoardItem{
			ID: iid, BoardID: bid, Kind: domain.BoardItemAsset, AssetID: aid,
			X:         float64(n%boardBatchCols) * stride,
			Y:         startY + float64(n/boardBatchCols)*stride,
			W:         boardItemDefaultSize,
			H:         boardItemDefaultSize,
			Rotation:  0,
			Z:         baseZ + n,
			CreatedAt: now,
		})
	}
	return items, nil
}

// notFoundOr500 maps a store lookup error to 404 when the entity is missing,
// otherwise 500. Shared by the board/asset validation in the batch handler.
func notFoundOr500(err error) int {
	if errors.Is(err, domain.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
