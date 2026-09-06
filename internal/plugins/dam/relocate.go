package dam

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
)

// relocateAsset repoints a single asset at a new storage path (manual relocate)
// and clears its missing flag.
// POST /api/dam/assets/{id}/relocate {"new_path": "...", "provider": "..."}
func (p *Plugin) relocateAsset(w http.ResponseWriter, r *http.Request) {
	rawID := chi.URLParam(r, "id")
	id, err := domain.NewAssetID(rawID)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid id: %w", err))
		return
	}

	var req struct {
		NewPath  string `json:"new_path"`
		Provider string `json:"provider"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.NewPath == "" {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("new_path is required"))
		return
	}
	// Never let an anchor point into the hetu-managed tree: it is excluded from
	// scans (issue #58), so an anchor there would be stranded as permanently
	// missing on the next scan.
	if domain.IsManagedPath(req.NewPath) {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("new_path must not be under %s", domain.ManagedDirName))
		return
	}

	// The asset must exist before it can be relocated.
	asset, err := p.k.Store.GetAsset(r.Context(), p.owner, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpjson.WriteError(w, http.StatusNotFound, err)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	// Default the provider to the asset's current one.
	provider := req.Provider
	if provider == "" {
		provider = asset.Provider
	}

	// The new path must be reachable on the target provider.
	sp, ok := p.k.Storage.Get(provider)
	if !ok {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("provider %q not found", provider))
		return
	}
	if _, err := sp.Stat(r.Context(), req.NewPath); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("new_path not accessible: %w", err))
		return
	}

	if err := p.k.Store.RelocateAsset(r.Context(), p.owner, id, provider, req.NewPath); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"relocated": rawID})
}

// rebaseRelocate batch-repoints assets by replacing a path prefix (e.g. after a
// whole folder is moved) and clears their missing flags.
// POST /api/dam/relocate/rebase {"old_prefix": "...", "new_prefix": "...", "provider": "local"}
func (p *Plugin) rebaseRelocate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPrefix string `json:"old_prefix"`
		NewPrefix string `json:"new_prefix"`
		Provider  string `json:"provider"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.OldPrefix == "" || req.NewPrefix == "" {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("old_prefix and new_prefix are required"))
		return
	}
	if req.Provider == "" {
		req.Provider = local.ProviderName
	}

	if err := p.k.Store.RebaseAssets(r.Context(), p.owner, req.Provider, req.OldPrefix, req.NewPrefix); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]bool{"rebased": true})
}
