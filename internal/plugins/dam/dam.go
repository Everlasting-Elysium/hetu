// Package dam is the digital-asset-management capability plugin: it exposes the
// asset library (list, later search/tags) over HTTP. Enabled via HETU_PLUGINS=dam.
package dam

import (
	"context"
	"net/http"
	"time"

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
		{Method: http.MethodGet, Pattern: "/search", Handler: p.searchAssets},
	}
}

func (p *Plugin) listAssets(w http.ResponseWriter, r *http.Request) {
	limit := httpjson.QueryInt(r, "limit", 50)
	offset := httpjson.QueryInt(r, "offset", 0)
	assets, err := p.k.Store.ListAssets(r.Context(), p.owner, limit, offset)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDTOs(assets))
}

type assetDTO struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Ext       string `json:"ext"`
	Size      int64  `json:"size"`
	Path      string `json:"path"`
	Thumb     string `json:"thumb"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	IndexedAt string `json:"indexed_at"`
}

func toDTOs(assets []domain.Asset) []assetDTO {
	out := make([]assetDTO, 0, len(assets))
	for _, a := range assets {
		out = append(out, assetDTO{
			ID:        a.ID.String(),
			Kind:      string(a.Kind),
			Name:      a.Name,
			Ext:       a.Ext,
			Size:      a.Size,
			Path:      a.StoragePath,
			Thumb:     a.ThumbPath,
			Width:     a.Width,
			Height:    a.Height,
			IndexedAt: a.IndexedAt.Format(time.RFC3339),
		})
	}
	return out
}
