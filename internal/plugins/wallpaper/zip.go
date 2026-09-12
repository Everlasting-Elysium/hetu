package wallpaper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/ziputil"
)

// maxZipItems is the wallpaper package's name for the shared bundle cap. It
// aliases ziputil.MaxItems rather than redefining the number, so the value
// lives in exactly one place while the local guard (and its test) keep the
// short local name.
const maxZipItems = ziputil.MaxItems

// downloadZip handles GET /api/wallpaper/download/zip?ids=a,b,c: it streams a
// zip of the requested assets (current-version bytes). Unresolvable ids (bad
// format, missing asset, unregistered provider) are skipped; 400 for an empty
// or over-cap id list, 404 when nothing resolves. The packaging itself (stream,
// per-item skip, name de-dup) is shared with DAM's batch export via ziputil.
func (p *Plugin) downloadZip(w http.ResponseWriter, r *http.Request) {
	ids := dedupIDs(r.URL.Query().Get("ids"))
	if len(ids) == 0 {
		httpjson.WriteError(w, http.StatusBadRequest, errors.New("ids must not be empty"))
		return
	}
	if len(ids) > maxZipItems {
		httpjson.WriteError(w, http.StatusBadRequest,
			fmt.Errorf("too many ids: %d exceeds max %d", len(ids), maxZipItems))
		return
	}
	items := p.resolveZipItems(r.Context(), ids)
	if len(items) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="wallpapers.zip"`)
	ziputil.Stream(r.Context(), p.k.Log, w, items)
}

// resolveZipItems maps raw ids to streamable items, silently dropping any id
// that does not parse, is not the owner's asset, or whose storage provider is
// not registered — leaving the empty-result 404 decision to the caller.
func (p *Plugin) resolveZipItems(ctx context.Context, ids []string) []ziputil.Item {
	items := make([]ziputil.Item, 0, len(ids))
	for _, s := range ids {
		aid, err := domain.NewAssetID(s)
		if err != nil {
			continue
		}
		asset, err := p.k.Store.GetAsset(ctx, p.owner, aid)
		if err != nil {
			continue
		}
		providerName, storagePath, _ := p.currentFile(ctx, asset)
		provider, ok := p.k.Storage.Get(providerName)
		if !ok {
			continue
		}
		items = append(items, ziputil.Item{Provider: provider, Path: storagePath, Name: downloadName(asset)})
	}
	return items
}

// dedupIDs splits a comma-separated id list, trims each token, and drops blanks
// and duplicates while preserving first-seen order.
func dedupIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}
