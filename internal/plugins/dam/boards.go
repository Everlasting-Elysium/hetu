package dam

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// ── DTOs ────────────────────────────────────────────────────────────────

type boardDTO struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	Items     []boardItemDTO `json:"items,omitempty"`
}

func toBoardDTO(b domain.Board) boardDTO {
	return boardDTO{
		ID:        b.ID.String(),
		Name:      b.Name,
		CreatedAt: b.CreatedAt.Format(time.RFC3339),
		UpdatedAt: b.UpdatedAt.Format(time.RFC3339),
	}
}

// ── Handlers ────────────────────────────────────────────────────────────

func (p *Plugin) createBoard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	id, err := newID()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	bid, _ := domain.NewBoardID(id)
	now := time.Now().UTC().Truncate(time.Second)
	b := domain.Board{ID: bid, Owner: p.owner, Name: req.Name, CreatedAt: now, UpdatedAt: now}
	if err := p.k.Store.CreateBoard(r.Context(), b); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toBoardDTO(b))
}

func (p *Plugin) listBoards(w http.ResponseWriter, r *http.Request) {
	boards, err := p.k.Store.ListBoards(r.Context(), p.owner)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]boardDTO, 0, len(boards))
	for _, b := range boards {
		out = append(out, toBoardDTO(b))
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (p *Plugin) getBoard(w http.ResponseWriter, r *http.Request) {
	bid, err := domain.NewBoardID(r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	b, err := p.k.Store.GetBoard(r.Context(), p.owner, bid)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpjson.WriteError(w, status, err)
		return
	}
	items, err := p.k.Store.ListBoardItems(r.Context(), bid)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	dto := toBoardDTO(b)
	dto.Items = make([]boardItemDTO, 0, len(items))
	for _, it := range items {
		dto.Items = append(dto.Items, toBoardItemDTO(it))
	}
	// Resolve each asset item's kind/name/thumb server-side (issue #86) so the
	// frontend can re-edit video/model items regardless of the asset panel's
	// current search/filter/pagination results.
	dto.Items = p.enrichItemAssets(r.Context(), dto.Items)
	httpjson.WriteJSON(w, http.StatusOK, dto)
}

func (p *Plugin) renameBoard(w http.ResponseWriter, r *http.Request) {
	bid, err := domain.NewBoardID(r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	if err := p.k.Store.UpdateBoardName(r.Context(), p.owner, bid, req.Name); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (p *Plugin) deleteBoard(w http.ResponseWriter, r *http.Request) {
	bid, err := domain.NewBoardID(r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.DeleteBoard(r.Context(), p.owner, bid); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
