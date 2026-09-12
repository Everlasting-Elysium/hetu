// Package wallpaper is the public, anonymous, read-only wallpaper capability
// plugin (issue #114): it exposes filtering/sorting/random/daily/collection
// browsing plus forced download and zip packaging over the very assets the DAM
// plugin already indexes. It adds no database tables and no auth of its own —
// enabling it via HETU_PLUGINS=...,wallpaper makes these endpoints world-
// readable by design, which is why the DTOs never leak storage_path/provider/
// folder_id/name. It mirrors DAM's New(owner) shape (no fixed provider) so each
// asset routes to its own storage backend on download.
package wallpaper

import (
	"context"
	"net/http"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// Name is the plugin's config key (HETU_PLUGINS). It is intentionally NOT in the
// HETU_PLUGINS default ("dam,nas"): wallpaper is opt-in because it is public.
const Name = "wallpaper"

// Plugin implements kernel.Plugin for the public wallpaper gallery.
type Plugin struct {
	k     *kernel.Kernel
	owner domain.OwnerID
	// now is the clock behind the deterministic /daily pick. It defaults to
	// time.Now; tests override it to assert a fixed date maps to a fixed asset.
	now func() time.Time
}

var _ kernel.Plugin = (*Plugin)(nil)

// New returns a wallpaper plugin scoped to owner. Unlike NAS it takes no fixed
// provider: wallpaper assets may live on different storage backends, so each
// download routes by the asset's own Provider (issue #114).
func New(owner domain.OwnerID) *Plugin {
	return &Plugin{owner: owner, now: time.Now}
}

// Name returns the plugin config key.
func (p *Plugin) Name() string { return Name }

// Init wires the plugin to the kernel.
func (p *Plugin) Init(_ context.Context, k *kernel.Kernel) error {
	p.k = k
	return nil
}

// Routes exposes the wallpaper API, mounted under /api/wallpaper. Every route is
// GET (read-only). Static first segments (/list,/random,/daily,/collections,
// /download) and the {id} wildcard coexist; asset ids are UUIDv7 so they never
// collide with a reserved word.
func (p *Plugin) Routes() []kernel.Route {
	return []kernel.Route{
		{Method: http.MethodGet, Pattern: "/list", Handler: p.list},
		{Method: http.MethodGet, Pattern: "/random", Handler: p.random},
		{Method: http.MethodGet, Pattern: "/daily", Handler: p.daily},
		{Method: http.MethodGet, Pattern: "/collections", Handler: p.listCollections},
		{Method: http.MethodGet, Pattern: "/collections/{id}", Handler: p.getCollectionAssets},
		{Method: http.MethodGet, Pattern: "/{id}/thumb", Handler: p.serveThumb},
		{Method: http.MethodGet, Pattern: "/{id}/download", Handler: p.download},
		{Method: http.MethodGet, Pattern: "/download/zip", Handler: p.downloadZip},
	}
}
