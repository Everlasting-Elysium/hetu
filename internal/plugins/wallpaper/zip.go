package wallpaper

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// maxZipItems caps how many assets one /download/zip request may bundle, so an
// anonymous caller cannot ask the server to stream the entire library at once.
const maxZipItems = 50

// zipItem is a resolved, ready-to-stream member of a zip request.
type zipItem struct {
	provider kernel.StorageProvider
	path     string
	name     string
}

// downloadZip handles GET /api/wallpaper/download/zip?ids=a,b,c: it streams a
// zip of the requested assets (current-version bytes). Unresolvable ids (bad
// format, missing asset, unregistered provider) are skipped; 400 for an empty
// or over-cap id list, 404 when nothing resolves. Because the archive streams,
// a per-asset open failure after the header is sent can only be skipped, not
// turned into an error status.
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
	p.streamZip(r.Context(), w, items)
}

// resolveZipItems maps raw ids to streamable items, silently dropping any id
// that does not parse, is not the owner's asset, or whose storage provider is
// not registered — leaving the empty-result 404 decision to the caller.
func (p *Plugin) resolveZipItems(ctx context.Context, ids []string) []zipItem {
	items := make([]zipItem, 0, len(ids))
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
		items = append(items, zipItem{provider: provider, path: storagePath, name: downloadName(asset)})
	}
	return items
}

// streamZip writes each item into a zip archive on w. A per-item open/create/
// copy failure is logged and skipped so one bad asset never aborts the whole
// download. Entry names are de-duplicated (only successfully-added entries
// consume a name slot) so two assets sharing a base name do not collide.
func (p *Plugin) streamZip(ctx context.Context, w http.ResponseWriter, items []zipItem) {
	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()
	seen := make(map[string]int, len(items))
	for _, it := range items {
		f, err := it.provider.Open(ctx, it.path)
		if err != nil {
			p.k.Log.WarnContext(ctx, "wallpaper zip: open asset", slog.String("path", it.path), slog.Any("err", err))
			continue
		}
		entry, err := zw.Create(uniqueName(seen, it.name))
		if err != nil {
			_ = f.Close()
			p.k.Log.WarnContext(ctx, "wallpaper zip: create entry", slog.Any("err", err))
			continue
		}
		if _, err := io.Copy(entry, f); err != nil {
			p.k.Log.WarnContext(ctx, "wallpaper zip: copy asset", slog.String("path", it.path), slog.Any("err", err))
		}
		_ = f.Close()
	}
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

// uniqueName returns name the first time it is seen and appends " (2)", " (3)",
// ... (before the extension) on each repeat, tracking counts in seen.
func uniqueName(seen map[string]int, name string) string {
	n := seen[name]
	seen[name]++
	if n == 0 {
		return name
	}
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s (%d)%s", base, n+1, ext)
}
