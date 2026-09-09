package dam

import (
	"errors"
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// collectionDTO is the wire form of a collection. Cover carries the resolved
// effective cover on list responses (explicit override, else lowest-ord member,
// else empty); on create/update it echoes the raw override the client set.
type collectionDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Cover    string `json:"cover"`
}

func toCollectionDTO(c domain.Collection) collectionDTO {
	return collectionDTO{ID: c.ID.String(), Name: c.Name, ParentID: c.ParentID, Cover: c.Cover}
}

func toCollectionDTOs(cols []domain.Collection) []collectionDTO {
	out := make([]collectionDTO, 0, len(cols))
	for _, c := range cols {
		out = append(out, toCollectionDTO(c))
	}
	return out
}

// createCollection makes an empty collection (cover is set later via PATCH once
// it has members, since a cover must reference a current member).
func (p *Plugin) createCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("collection name required"))
		return
	}
	raw, err := newID()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	id, err := domain.NewCollectionID(raw)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	col := domain.Collection{ID: id, Owner: p.owner, ParentID: req.ParentID, Name: req.Name}
	if err := p.k.Store.CreateCollection(r.Context(), col); err != nil {
		httpjson.WriteError(w, collectionErrStatus(err), err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toCollectionDTO(col))
}

func (p *Plugin) listCollections(w http.ResponseWriter, r *http.Request) {
	cols, err := p.k.Store.ListCollections(r.Context(), p.owner)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toCollectionDTOs(cols))
}

// updateCollection patches name/parent_id/cover, applying only the fields the
// request provides (pointer = present). The current row supplies the untouched
// fields, and GetCollection's raw cover keeps an unrelated PATCH from freezing a
// fallback cover as an explicit override.
func (p *Plugin) updateCollection(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewCollectionID(r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var req struct {
		Name     *string `json:"name"`
		ParentID *string `json:"parent_id"`
		Cover    *string `json:"cover"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	cur, err := p.k.Store.GetCollection(r.Context(), p.owner, id)
	if err != nil {
		httpjson.WriteError(w, collectionErrStatus(err), err)
		return
	}
	name := cur.Name
	if req.Name != nil {
		name = *req.Name
	}
	if name == "" {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("collection name required"))
		return
	}
	parentID := cur.ParentID
	if req.ParentID != nil {
		parentID = *req.ParentID
	}
	cover := cur.Cover
	if req.Cover != nil {
		cover = *req.Cover
	}
	if err := p.k.Store.UpdateCollection(r.Context(), p.owner, id, name, parentID, cover); err != nil {
		httpjson.WriteError(w, collectionErrStatus(err), err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (p *Plugin) deleteCollection(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewCollectionID(r.PathValue("id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.DeleteCollection(r.Context(), p.owner, id); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// collectionErrStatus maps a store error to an HTTP status: ErrNotFound (missing
// collection/parent, or a cover that is not a member) is 404, ErrCollectionCycle
// (a parent_id that would make a collection its own ancestor) is 400, everything
// else 500.
func collectionErrStatus(err error) int {
	switch {
	case errors.Is(err, domain.ErrCollectionCycle):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
