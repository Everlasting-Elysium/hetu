// Package eagle reads an Eagle (.library) digital-asset library strictly
// read-only and yields hetu import items. Eagle stores one directory per item
// at images/<id>.info/ holding the original file plus a metadata.json; the
// library's folder tree lives in <root>/metadata.json. See docs/importers.md
// for the field mapping.
package eagle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/importers"
)

// Source reads an Eagle library rooted at a .library directory. It holds no open
// file handle, so it is safe to use from a deferred/async import job.
type Source struct {
	root       string              // absolute path to the .library dir
	folderPath map[string][]string // Eagle folder id → root→leaf name segments
}

// libMeta is the subset of <root>/metadata.json we consume: the folder tree.
type libMeta struct {
	Folders []folderNode `json:"folders"`
}

type folderNode struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Children []folderNode `json:"children"`
}

// itemMeta is the subset of images/<id>.info/metadata.json we consume.
type itemMeta struct {
	Name       string   `json:"name"`
	Ext        string   `json:"ext"`
	Tags       []string `json:"tags"`
	Folders    []string `json:"folders"`
	Annotation string   `json:"annotation"`
	URL        string   `json:"url"`
	Star       int      `json:"star"`
	BTime      int64    `json:"btime"` // milliseconds since epoch
	IsDeleted  bool     `json:"isDeleted"`
}

// Open validates that dir looks like an Eagle library and pre-reads its folder
// tree so item folder ids resolve to paths.
func Open(dir string) (*Source, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("eagle: resolve %q: %w", dir, err)
	}
	if fi, err := os.Stat(filepath.Join(root, "images")); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("eagle: %q is not a .library (missing images/)", dir)
	}
	s := &Source{root: root, folderPath: map[string][]string{}}
	s.loadFolders()
	return s, nil
}

// Kind returns importers.KindEagle.
func (s *Source) Kind() importers.SourceKind { return importers.KindEagle }

// loadFolders reads the library folder tree (best effort: an absent or malformed
// tree simply means items resolve to no folder).
func (s *Source) loadFolders() {
	data, err := os.ReadFile(filepath.Join(s.root, "metadata.json"))
	if err != nil {
		return
	}
	var m libMeta
	if json.Unmarshal(data, &m) != nil {
		return
	}
	var walk func(nodes []folderNode, prefix []string)
	walk = func(nodes []folderNode, prefix []string) {
		for _, n := range nodes {
			path := make([]string, len(prefix)+1)
			copy(path, prefix)
			path[len(prefix)] = n.Name
			s.folderPath[n.ID] = path
			walk(n.Children, path)
		}
	}
	walk(m.Folders, nil)
}

// Each streams every live (non-deleted) item to fn.
func (s *Source) Each(ctx context.Context, fn func(importers.ImportItem) error) error {
	entries, err := os.ReadDir(filepath.Join(s.root, "images"))
	if err != nil {
		return fmt.Errorf("eagle: read images: %w", err)
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !e.IsDir() || !strings.HasSuffix(e.Name(), ".info") {
			continue
		}
		item, ok := s.readItem(e.Name())
		if !ok {
			continue
		}
		if err := fn(item); err != nil {
			return err
		}
	}
	return nil
}

// readItem parses one images/<infoDir>/metadata.json into an ImportItem,
// returning ok=false when the item is deleted, unreadable, or its media file is
// missing (skipped, not fatal).
func (s *Source) readItem(infoDir string) (importers.ImportItem, bool) {
	dir := filepath.Join(s.root, "images", infoDir)
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return importers.ImportItem{}, false
	}
	var m itemMeta
	if json.Unmarshal(data, &m) != nil || m.IsDeleted || m.Name == "" {
		return importers.ImportItem{}, false
	}
	mediaName := m.Name
	if m.Ext != "" {
		mediaName += "." + m.Ext
	}
	mediaPath := filepath.Join(dir, mediaName)
	if _, err := os.Stat(mediaPath); err != nil {
		return importers.ImportItem{}, false
	}
	item := importers.ImportItem{
		AbsPath:   mediaPath,
		Name:      mediaName,
		Rating:    m.Star,
		Note:      m.Annotation,
		SourceURL: m.URL,
		Tags:      flatTags(m.Tags),
		Folders:   s.resolveFolders(m.Folders),
	}
	if m.BTime > 0 {
		item.CreatedAt = time.UnixMilli(m.BTime).UTC()
	}
	return item, true
}

// flatTags maps Eagle's flat tag strings to single-segment tag paths: Eagle tags
// are flat labels (a "/" is part of the name, not a hierarchy separator).
func flatTags(tags []string) []importers.NamePath {
	out := make([]importers.NamePath, 0, len(tags))
	for _, t := range tags {
		if strings.TrimSpace(t) != "" {
			out = append(out, importers.NamePath{t})
		}
	}
	return out
}

// resolveFolders maps Eagle folder ids to hetu folder paths via the library
// tree; unknown ids are dropped.
func (s *Source) resolveFolders(ids []string) []importers.NamePath {
	out := make([]importers.NamePath, 0, len(ids))
	for _, id := range ids {
		if segs, ok := s.folderPath[id]; ok {
			out = append(out, importers.NamePath(segs))
		}
	}
	return out
}
