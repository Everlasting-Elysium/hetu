package dam

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// updateNote handles PUT /assets/{id}/note — upserts the manual-layer caption.
func (p *Plugin) updateNote(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("text must not be empty"))
		return
	}
	if err := p.k.Store.UpsertManualCaption(r.Context(), p.owner, id, text); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpjson.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"note": text})
}

// deleteNote handles DELETE /assets/{id}/note — removes the manual-layer caption.
func (p *Plugin) deleteNote(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.DeleteManualCaption(r.Context(), p.owner, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpjson.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
