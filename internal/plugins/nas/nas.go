// Package nas is the NAS capability plugin: it exposes filesystem browsing,
// file download, and share links over the kernel's storage abstraction.
// Enabled via HETU_PLUGINS=nas.
package nas

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// Name is the plugin's config key (HETU_PLUGINS).
const Name = "nas"

// Plugin implements kernel.Plugin for NAS-style browsing, download, and shares.
type Plugin struct {
	k        *kernel.Kernel
	owner    domain.OwnerID
	provider string
}

var (
	_ kernel.Plugin         = (*Plugin)(nil)
	_ kernel.TopLevelRouter = (*Plugin)(nil)
)

// New returns a NAS plugin scoped to owner, backed by the named storage provider.
func New(owner domain.OwnerID, provider string) *Plugin {
	return &Plugin{owner: owner, provider: provider}
}

// Name returns the plugin config key.
func (p *Plugin) Name() string { return Name }

// Init wires the plugin to the kernel.
func (p *Plugin) Init(_ context.Context, k *kernel.Kernel) error {
	p.k = k
	return nil
}

// Routes exposes the NAS API, mounted under /api/nas.
func (p *Plugin) Routes() []kernel.Route {
	return []kernel.Route{
		{Method: http.MethodGet, Pattern: "/browse", Handler: p.browse},
		{Method: http.MethodGet, Pattern: "/download", Handler: p.download},
		{Method: http.MethodPost, Pattern: "/shares", Handler: p.createShare},
	}
}

// TopLevelRoutes registers the public share endpoint at the router root.
func (p *Plugin) TopLevelRoutes() []kernel.Route {
	return []kernel.Route{
		{Method: http.MethodGet, Pattern: "/s/{token}", Handler: p.accessShare},
	}
}

func (p *Plugin) browse(w http.ResponseWriter, r *http.Request) {
	provider, ok := p.k.Storage.Get(p.provider)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError,
			fmt.Errorf("storage provider %q not registered", p.provider))
		return
	}
	entries, err := provider.List(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDTOs(entries))
}

type entryDTO struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

func toDTOs(entries []domain.Entry) []entryDTO {
	out := make([]entryDTO, 0, len(entries))
	for _, e := range entries {
		// Hide the hetu-managed tree (version copies, issue #58) so managed
		// internals never surface to users browsing the library.
		if e.Name == domain.ManagedDirName {
			continue
		}
		out = append(out, entryDTO{Name: e.Name, Path: e.Path, IsDir: e.IsDir, Size: e.Size})
	}
	return out
}
