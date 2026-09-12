package dam

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/ziputil"
)

// batchExport handles POST /api/dam/batch/export with body {"asset_ids": [...]}:
// it streams a zip of the selected assets' current-version bytes (issue #62).
// Unlike wallpaper's anonymous GET /download/zip?ids=, this packages the owner's
// selection set, so it is POST + JSON body like every other /batch/* endpoint
// and each id is owner-scoped via GetAsset. Cross-owner, unknown, or unregistered-
// provider ids are silently skipped; an empty or over-cap list is 400, and 404
// when nothing resolves. Once the archive streams a per-asset open failure can
// only be skipped (ziputil.Stream), never turned into an error status.
func (p *Plugin) batchExport(w http.ResponseWriter, r *http.Request) {
	ids, ok := p.decodeAssetIDs(w, r)
	if !ok {
		return
	}
	if len(ids) > ziputil.MaxItems {
		httpjson.WriteError(w, http.StatusBadRequest,
			fmt.Errorf("too many asset_ids: %d exceeds max %d", len(ids), ziputil.MaxItems))
		return
	}
	items := p.resolveExportItems(r.Context(), ids)
	if len(items) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="assets.zip"`)
	ziputil.Stream(r.Context(), p.k.Log, w, items)
}

// resolveExportItems maps owner-scoped asset ids to streamable zip items,
// silently dropping any id that is not the owner's asset (GetAsset is scoped by
// owner, so a cross-owner id resolves to not-found) or whose storage provider is
// not registered — leaving the empty-result 404 to the caller. The current
// version's bytes are served (issue #58) by resolving through currentVersionFile,
// mirroring serveFile; the entry name is the display name when set, else the
// indexed file name.
func (p *Plugin) resolveExportItems(ctx context.Context, ids []domain.AssetID) []ziputil.Item {
	items := make([]ziputil.Item, 0, len(ids))
	for _, aid := range ids {
		asset, err := p.k.Store.GetAsset(ctx, p.owner, aid)
		if err != nil {
			continue
		}
		providerName, storagePath := asset.Provider, asset.StoragePath
		if v, ok := p.currentVersionFile(ctx, asset); ok {
			providerName, storagePath = v.Provider, v.StoragePath
		}
		provider, ok := p.k.Storage.Get(providerName)
		if !ok {
			continue
		}
		name := asset.Name
		if asset.DisplayName != "" {
			name = asset.DisplayName
		}
		items = append(items, ziputil.Item{Provider: provider, Path: storagePath, Name: name})
	}
	return items
}
