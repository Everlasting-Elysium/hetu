package dam

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// folderDTO is the wire form of a folder. Cover carries the resolved effective
// cover asset id on list responses (explicit override, else the earliest-indexed
// live asset, else empty); CoverURL is that asset's /thumb endpoint when it
// exists and has a thumbnail, otherwise omitted. Color is a hex label, e.g.
// "#FF5733", styled like a tag color.
type folderDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Path     string `json:"path"`
	Cover    string `json:"cover"`
	CoverURL string `json:"cover_url,omitempty"`
	Color    string `json:"color"`
}

func (p *Plugin) toFolderDTO(ctx context.Context, f domain.Folder) folderDTO {
	dto := folderDTO{
		ID:       f.ID.String(),
		Name:     f.Name,
		ParentID: f.ParentID,
		Path:     f.Path,
		Cover:    f.Cover,
		Color:    f.Color,
	}
	if url, ok := p.folderCoverURL(ctx, f.Cover); ok {
		dto.CoverURL = url
	}
	return dto
}

func (p *Plugin) toFolderDTOs(ctx context.Context, folders []domain.Folder) []folderDTO {
	out := make([]folderDTO, 0, len(folders))
	for _, f := range folders {
		out = append(out, p.toFolderDTO(ctx, f))
	}
	return out
}

// folderCoverURL resolves a cover asset id into its /thumb URL. ok is false
// (cover_url omitted, never an error) when the id is empty, unparseable, missing,
// or the asset has no thumbnail — a broken cover must not fail the whole listing.
func (p *Plugin) folderCoverURL(ctx context.Context, cover string) (string, bool) {
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
	return fmt.Sprintf("/api/dam/assets/%s/thumb", aid.String()), true
}

func (p *Plugin) createFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
		Path     string `json:"path"`
		Cover    string `json:"cover"`
		Color    string `json:"color"`
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
	folder := domain.Folder{
		ID: id, Owner: p.owner, ParentID: req.ParentID,
		Name: req.Name, Path: path, Cover: req.Cover, Color: req.Color,
	}
	if err := p.k.Store.CreateFolder(r.Context(), folder); err != nil {
		httpjson.WriteError(w, folderErrStatus(err), err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, p.toFolderDTO(r.Context(), folder))
}

func (p *Plugin) listFolders(w http.ResponseWriter, r *http.Request) {
	folders, err := p.k.Store.ListFolders(r.Context(), p.owner)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, p.toFolderDTOs(r.Context(), folders))
}

// updateFolder patches a folder's cover/color, applying only the fields the
// request provides (pointer = present). The current row supplies the untouched
// fields, and GetFolder's raw cover keeps an unrelated PATCH from freezing a
// fallback cover as an explicit override.
func (p *Plugin) updateFolder(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewFolderID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var req struct {
		Cover *string `json:"cover"`
		Color *string `json:"color"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	cur, err := p.k.Store.GetFolder(r.Context(), p.owner, id)
	if err != nil {
		httpjson.WriteError(w, folderErrStatus(err), err)
		return
	}
	cover := cur.Cover
	if req.Cover != nil {
		cover = *req.Cover
	}
	color := cur.Color
	if req.Color != nil {
		color = *req.Color
	}
	if err := p.k.Store.UpdateFolderCover(r.Context(), p.owner, id, cover, color); err != nil {
		httpjson.WriteError(w, folderErrStatus(err), err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
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

// folderErrStatus maps a store error to an HTTP status: ErrNotFound (missing
// folder, or a cover that is not a live owner-scoped asset) is 404, everything
// else 500.
func folderErrStatus(err error) int {
	if errors.Is(err, domain.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
