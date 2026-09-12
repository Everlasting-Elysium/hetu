// Package ziputil streams a set of storage-backed files into a zip archive on
// the fly. It is shared by the wallpaper plugin's GET /download/zip and the DAM
// plugin's POST /batch/export so the two never drift on the packaging rules:
// a per-item open/create/copy failure is skipped (an archive already streaming
// can no longer send an error status), and colliding entry names are
// de-duplicated. It depends only on the kernel StorageProvider contract, so
// each plugin resolves its own items (by id list vs. selection set) and hands
// them here to stream. Mirrors internal/httpjson: a small, dependency-light
// helper package shared by plugins.
package ziputil

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// MaxItems caps how many assets one packaging request may bundle, so a caller
// cannot ask the server to stream the entire library at once. Shared by every
// zip endpoint (wallpaper download/zip, DAM batch/export) so the cap is one
// number, not a per-endpoint magic constant.
const MaxItems = 50

// Item is a resolved, ready-to-stream member of a zip request: an openable path
// on a storage provider plus the entry name it should carry in the archive.
type Item struct {
	Provider kernel.StorageProvider
	Path     string
	Name     string
}

// Stream writes each item into a zip archive on w. A per-item open/create/copy
// failure is logged and skipped so one bad asset never aborts the whole
// download — once the archive has started streaming, an error status can no
// longer be sent, so skipping is the only safe recovery. Entry names are
// de-duplicated (only successfully-added entries consume a name slot) so two
// assets sharing a base name do not collide.
func Stream(ctx context.Context, log *slog.Logger, w io.Writer, items []Item) {
	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()
	seen := make(map[string]int, len(items))
	for _, it := range items {
		f, err := it.Provider.Open(ctx, it.Path)
		if err != nil {
			log.WarnContext(ctx, "zip: open item", slog.String("path", it.Path), slog.Any("err", err))
			continue
		}
		entry, err := zw.Create(uniqueName(seen, it.Name))
		if err != nil {
			_ = f.Close()
			log.WarnContext(ctx, "zip: create entry", slog.Any("err", err))
			continue
		}
		if _, err := io.Copy(entry, f); err != nil {
			log.WarnContext(ctx, "zip: copy item", slog.String("path", it.Path), slog.Any("err", err))
		}
		_ = f.Close()
	}
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
