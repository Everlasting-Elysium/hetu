package dam

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

type tagDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	ParentID string `json:"parent_id"`
}

func toTagDTO(t domain.Tag) tagDTO {
	return tagDTO{ID: t.ID.String(), Name: t.Name, Color: t.Color, ParentID: t.ParentID}
}

func toTagDTOs(tags []domain.Tag) []tagDTO {
	out := make([]tagDTO, 0, len(tags))
	for _, t := range tags {
		out = append(out, toTagDTO(t))
	}
	return out
}

func (p *Plugin) createTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Color    string `json:"color"`
		ParentID string `json:"parent_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("tag name required"))
		return
	}
	raw, err := newID()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	id, err := domain.NewTagID(raw)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	tag := domain.Tag{ID: id, Owner: p.owner, ParentID: req.ParentID, Name: req.Name, Color: req.Color}
	if err := p.k.Store.CreateTag(r.Context(), tag); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toTagDTO(tag))
}

func (p *Plugin) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := p.k.Store.ListTags(r.Context(), p.owner)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toTagDTOs(tags))
}

func (p *Plugin) deleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewTagID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.DeleteTag(r.Context(), p.owner, id); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (p *Plugin) batchTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AssetIDs []string `json:"asset_ids"`
		TagIDs   []string `json:"tag_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	assetIDs, err := parseAssetIDs(req.AssetIDs)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	tagIDs, err := parseTagIDs(req.TagIDs)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.BatchAddTags(r.Context(), p.owner, assetIDs, tagIDs); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"tagged": len(assetIDs)})
}

func (p *Plugin) batchUntag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AssetIDs []string `json:"asset_ids"`
		TagID    string   `json:"tag_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	assetIDs, err := parseAssetIDs(req.AssetIDs)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	tagID, err := domain.NewTagID(req.TagID)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.BatchRemoveTags(r.Context(), p.owner, assetIDs, tagID); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"untagged": len(assetIDs)})
}

func (p *Plugin) assetTags(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	tags, err := p.k.Store.ListAssetTags(r.Context(), id)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toTagDTOs(tags))
}

// parseTagIDs parses a non-empty list of raw tag ids into domain IDs.
func parseTagIDs(raw []string) ([]domain.TagID, error) {
	if len(raw) == 0 {
		return nil, errors.New("tag_ids must not be empty")
	}
	ids := make([]domain.TagID, 0, len(raw))
	for _, s := range raw {
		id, err := domain.NewTagID(s)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
