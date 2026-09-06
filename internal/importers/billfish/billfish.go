// Package billfish reads a Billfish library's SQLite catalog (.bf/billfish.db)
// strictly read-only and yields hetu import items. The DB is opened with
// mode=ro&immutable=1 so migration never modifies the source library. See
// docs/importers.md for the field mapping.
package billfish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/importers"
)

// Source reads a Billfish library rooted at dir (which contains .bf/billfish.db).
// It holds no open handle between calls; Each opens and closes the DB, so the
// Source is safe to run from a deferred/async import job.
type Source struct {
	root   string // library root; media paths are relative to it
	dbPath string // absolute path to .bf/billfish.db
}

// Open validates that dir contains a Billfish catalog.
func Open(dir string) (*Source, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("billfish: resolve %q: %w", dir, err)
	}
	dbPath := filepath.Join(root, ".bf", "billfish.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("billfish: %q is not a library (missing .bf/billfish.db): %w", dir, err)
	}
	return &Source{root: root, dbPath: dbPath}, nil
}

// Kind returns importers.KindBillfish.
func (s *Source) Kind() importers.SourceKind { return importers.KindBillfish }

// Each opens the catalog read-only, resolves the folder and tag trees, then
// streams one item per live (non-recycled) file. The DB is closed before Each
// returns.
func (s *Source) Each(ctx context.Context, fn func(importers.ImportItem) error) error {
	db, err := openRO(s.dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	cat, err := loadCatalog(ctx, db)
	if err != nil {
		return err
	}
	files, err := cat.queryFiles(ctx, db)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, ok := s.toItem(f, cat)
		if !ok {
			continue
		}
		if err := fn(item); err != nil {
			return err
		}
	}
	return nil
}

// toItem builds an ImportItem from a file row, resolving its media path, folder
// path (via bf_file.pid), and tag paths; ok=false if the media file is absent.
func (s *Source) toItem(f fileRow, cat *catalog) (importers.ImportItem, bool) {
	abs := f.path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.root, filepath.FromSlash(f.path))
	}
	if _, err := os.Stat(abs); err != nil {
		return importers.ImportItem{}, false
	}
	name := f.name
	if f.ext != "" {
		name += "." + f.ext
	}
	item := importers.ImportItem{
		AbsPath:   abs,
		Name:      name,
		Rating:    f.score,
		Note:      f.note,
		SourceURL: f.origin,
		Tags:      cat.tagPaths(f.id),
		Folders:   cat.folderPaths(f.pid),
	}
	if f.createTime > 0 {
		item.CreatedAt = time.Unix(f.createTime, 0).UTC()
	}
	return item, true
}
