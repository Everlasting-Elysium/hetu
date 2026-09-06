// Package dam is the digital-asset-management capability plugin: it exposes the
// asset library (list, batch operations, tags, folders, trash) over HTTP.
// Enabled via HETU_PLUGINS=dam.
package dam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// Name is the plugin's config key (HETU_PLUGINS).
const Name = "dam"

// Plugin implements kernel.Plugin for asset management.
type Plugin struct {
	k     *kernel.Kernel
	owner domain.OwnerID
}

var _ kernel.Plugin = (*Plugin)(nil)

// New returns a DAM plugin scoped to owner.
func New(owner domain.OwnerID) *Plugin { return &Plugin{owner: owner} }

// Name returns the plugin config key.
func (p *Plugin) Name() string { return Name }

// Init wires the plugin to the kernel.
func (p *Plugin) Init(_ context.Context, k *kernel.Kernel) error {
	p.k = k
	return nil
}

// Routes exposes the DAM API, mounted under /api/dam.
func (p *Plugin) Routes() []kernel.Route {
	return []kernel.Route{
		{Method: http.MethodGet, Pattern: "/assets", Handler: p.listAssets},
		{Method: http.MethodGet, Pattern: "/assets/{id}/tags", Handler: p.assetTags},
		{Method: http.MethodGet, Pattern: "/assets/{id}/thumb", Handler: p.serveThumb},
		// /search dispatches on query params: ?q= full-text (FTS5), ?color= palette.
		{Method: http.MethodGet, Pattern: "/search", Handler: p.search},

		{Method: http.MethodPost, Pattern: "/batch/rate", Handler: p.batchRate},
		{Method: http.MethodPost, Pattern: "/batch/color", Handler: p.batchColor},
		{Method: http.MethodPost, Pattern: "/batch/rename", Handler: p.batchRename},
		{Method: http.MethodPost, Pattern: "/batch/move", Handler: p.batchMove},
		{Method: http.MethodPost, Pattern: "/batch/trash", Handler: p.batchTrash},
		{Method: http.MethodPost, Pattern: "/batch/restore", Handler: p.batchRestore},
		{Method: http.MethodPost, Pattern: "/batch/tag", Handler: p.batchTag},
		{Method: http.MethodPost, Pattern: "/batch/untag", Handler: p.batchUntag},

		{Method: http.MethodGet, Pattern: "/trash", Handler: p.listTrash},
		{Method: http.MethodDelete, Pattern: "/trash", Handler: p.emptyTrash},

		{Method: http.MethodPost, Pattern: "/tags", Handler: p.createTag},
		{Method: http.MethodGet, Pattern: "/tags", Handler: p.listTags},
		{Method: http.MethodDelete, Pattern: "/tags/{id}", Handler: p.deleteTag},

		{Method: http.MethodPost, Pattern: "/folders", Handler: p.createFolder},
		{Method: http.MethodGet, Pattern: "/folders", Handler: p.listFolders},
		{Method: http.MethodDelete, Pattern: "/folders/{id}", Handler: p.deleteFolder},

		{Method: http.MethodGet, Pattern: "/duplicates", Handler: p.listDuplicates},
		{Method: http.MethodGet, Pattern: "/duplicates/similar", Handler: p.listSimilar},

		{Method: http.MethodPost, Pattern: "/boards", Handler: p.createBoard},
		{Method: http.MethodGet, Pattern: "/boards", Handler: p.listBoards},
		{Method: http.MethodGet, Pattern: "/boards/{id}", Handler: p.getBoard},
		{Method: http.MethodPatch, Pattern: "/boards/{id}", Handler: p.renameBoard},
		{Method: http.MethodDelete, Pattern: "/boards/{id}", Handler: p.deleteBoard},
		{Method: http.MethodPost, Pattern: "/boards/{id}/items", Handler: p.addBoardItem},
		{Method: http.MethodPatch, Pattern: "/boards/{id}/items", Handler: p.updateBoardItems},
		{Method: http.MethodDelete, Pattern: "/boards/{id}/items/{itemId}", Handler: p.deleteBoardItem},
	}
}

// listAssets lists the owner's live assets, optionally narrowed by
// ?folder=<id>, ?tag=<id>, and ?rating=<min> (0-5, keeps that many stars and
// up). Paged by ?limit=/?offset=.
func (p *Plugin) listAssets(w http.ResponseWriter, r *http.Request) {
	limit := httpjson.QueryInt(r, "limit", 50)
	offset := httpjson.QueryInt(r, "offset", 0)
	filter := domain.AssetFilter{
		FolderID:  r.URL.Query().Get("folder"),
		TagID:     r.URL.Query().Get("tag"),
		MinRating: httpjson.QueryInt(r, "rating", 0),
	}
	assets, err := p.k.Store.ListAssetsFiltered(r.Context(), p.owner, filter, limit, offset)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDTOs(assets))
}

type assetDTO struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Ext         string `json:"ext"`
	Size        int64  `json:"size"`
	Path        string `json:"path"`
	Thumb       string `json:"thumb"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	IndexedAt   string `json:"indexed_at"`
	Rating      int    `json:"rating"`
	Color       string `json:"color"`
	DisplayName string `json:"display_name"`
	FolderID    string `json:"folder_id"`
	DeletedAt   string `json:"deleted_at,omitempty"`
}

func toDTOs(assets []domain.Asset) []assetDTO {
	out := make([]assetDTO, 0, len(assets))
	for _, a := range assets {
		out = append(out, toDTO(a))
	}
	return out
}

func toDTO(a domain.Asset) assetDTO {
	dto := assetDTO{
		ID:          a.ID.String(),
		Kind:        string(a.Kind),
		Name:        a.Name,
		Ext:         a.Ext,
		Size:        a.Size,
		Path:        a.StoragePath,
		Thumb:       a.ThumbPath,
		Width:       a.Width,
		Height:      a.Height,
		IndexedAt:   a.IndexedAt.Format(time.RFC3339),
		Rating:      a.Rating,
		Color:       a.Color,
		DisplayName: a.DisplayName,
		FolderID:    a.FolderID,
	}
	if a.DeletedAt != nil {
		dto.DeletedAt = a.DeletedAt.Format(time.RFC3339)
	}
	return dto
}

// decodeJSON decodes the request body into dst, writing a 400 on failure.
// It returns true on success.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return false
	}
	return true
}

// newID generates a UUIDv7 string for a new entity (tag, folder).
func newID() (string, error) {
	u, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return u.String(), nil
}

// parseAssetIDs parses a non-empty list of raw asset ids into domain IDs.
func parseAssetIDs(raw []string) ([]domain.AssetID, error) {
	if len(raw) == 0 {
		return nil, errors.New("asset_ids must not be empty")
	}
	ids := make([]domain.AssetID, 0, len(raw))
	for _, s := range raw {
		id, err := domain.NewAssetID(s)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
