package dam

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

type folderDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Path     string `json:"path"`
}

func toFolderDTO(f domain.Folder) folderDTO {
	return folderDTO{ID: f.ID.String(), Name: f.Name, ParentID: f.ParentID, Path: f.Path}
}

func toFolderDTOs(folders []domain.Folder) []folderDTO {
	out := make([]folderDTO, 0, len(folders))
	for _, f := range folders {
		out = append(out, toFolderDTO(f))
	}
	return out
}

func (p *Plugin) createFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
		Path     string `json:"path"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("folder name required"))
		return
	}
	path := req.Path
	if path == "" {
		path = req.Name
	}
	raw, err := newID()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	id, err := domain.NewFolderID(raw)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	folder := domain.Folder{ID: id, Owner: p.owner, ParentID: req.ParentID, Name: req.Name, Path: path}
	if err := p.k.Store.CreateFolder(r.Context(), folder); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toFolderDTO(folder))
}

func (p *Plugin) listFolders(w http.ResponseWriter, r *http.Request) {
	folders, err := p.k.Store.ListFolders(r.Context(), p.owner)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toFolderDTOs(folders))
}

func (p *Plugin) deleteFolder(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewFolderID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.k.Store.DeleteFolder(r.Context(), p.owner, id); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
