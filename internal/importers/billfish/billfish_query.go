package billfish

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/Everlasting-Elysium/hetu/internal/importers"
)

// openRO opens the catalog read-only and immutable, so migration can never
// modify the source library (no writes, no lock files, no WAL creation).
func openRO(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&immutable=1")
	if err != nil {
		return nil, fmt.Errorf("billfish: open %q: %w", dbPath, err)
	}
	return db, nil
}

// fileRow is one row of the bf_file join used to build an import item.
type fileRow struct {
	id         int64
	name       string
	ext        string
	path       string
	pid        int64 // folder id (bf_folder.id); 0 = root
	createTime int64 // unix seconds
	score      int   // rating 0-5
	note       string
	origin     string // source URL
}

// catalog holds the folder and tag trees plus file→tag links, loaded once per
// Each so per-file resolution is in-memory.
type catalog struct {
	tagTable     string
	materialView string
	folderName   map[int64]string
	folderParent map[int64]int64
	tagName      map[int64]string
	tagParent    map[int64]int64
	fileTags     map[int64][]int64
}

// loadCatalog detects the schema version (v2 vs v1 tables) and loads the folder
// tree, tag tree, and file→tag links.
func loadCatalog(ctx context.Context, db *sql.DB) (*catalog, error) {
	c := &catalog{
		tagTable:     pick(ctx, db, "bf_tag_v2", "bf_tag"),
		materialView: pick(ctx, db, "bf_material_v2", "bf_material"),
		folderName:   map[int64]string{},
		folderParent: map[int64]int64{},
		tagName:      map[int64]string{},
		tagParent:    map[int64]int64{},
		fileTags:     map[int64][]int64{},
	}
	if err := loadTree(ctx, db, "bf_folder", c.folderName, c.folderParent); err != nil {
		return nil, err
	}
	if err := loadTree(ctx, db, c.tagTable, c.tagName, c.tagParent); err != nil {
		return nil, err
	}
	if err := c.loadFileTags(ctx, db); err != nil {
		return nil, err
	}
	return c, nil
}

// tableExists reports whether a table is present, so schema variance across
// Billfish versions (missing folder/tag tables) degrades gracefully.
func tableExists(ctx context.Context, db *sql.DB, name string) bool {
	var n int
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&n)
	return n > 0
}

// pick returns primary if that table exists, else fallback.
func pick(ctx context.Context, db *sql.DB, primary, fallback string) string {
	if tableExists(ctx, db, primary) {
		return primary
	}
	return fallback
}

// loadTree reads an (id, name, pid) hierarchy table into name/parent maps. A
// missing table is not an error: the library simply has no such hierarchy.
func loadTree(ctx context.Context, db *sql.DB, table string, name map[int64]string, parent map[int64]int64) error {
	if !tableExists(ctx, db, table) {
		return nil
	}
	rows, err := db.QueryContext(ctx, "SELECT id, name, pid FROM "+table)
	if err != nil {
		return fmt.Errorf("billfish: read %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, pid int64
		var nm string
		if err := rows.Scan(&id, &nm, &pid); err != nil {
			return fmt.Errorf("billfish: scan %s: %w", table, err)
		}
		name[id] = nm
		parent[id] = pid
	}
	return rows.Err()
}

// loadFileTags reads the file→tag join into fileTags. A missing join table is
// not an error: the library simply has no tag assignments.
func (c *catalog) loadFileTags(ctx context.Context, db *sql.DB) error {
	if !tableExists(ctx, db, "bf_tag_join_file") {
		return nil
	}
	rows, err := db.QueryContext(ctx, "SELECT file_id, tag_id FROM bf_tag_join_file")
	if err != nil {
		return fmt.Errorf("billfish: read bf_tag_join_file: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var fileID, tagID int64
		if err := rows.Scan(&fileID, &tagID); err != nil {
			return fmt.Errorf("billfish: scan bf_tag_join_file: %w", err)
		}
		c.fileTags[fileID] = append(c.fileTags[fileID], tagID)
	}
	return rows.Err()
}

// queryFiles joins bf_file with the material view (dimensions/recycle flag) and
// user data (rating/note/origin), excluding recycled files.
func (c *catalog) queryFiles(ctx context.Context, db *sql.DB) ([]fileRow, error) {
	q := `SELECT f.id, f.name, f.ext, f.path, f.pid, f.create_time,
	             COALESCE(mu.score, 0), COALESCE(mu.note, ''), COALESCE(mu.origin, '')
	      FROM bf_file f
	      LEFT JOIN ` + c.materialView + ` m ON m.file_id = f.id
	      LEFT JOIN bf_material_userdata mu ON mu.file_id = f.id
	      WHERE COALESCE(m.is_recycle, 0) = 0`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("billfish: query files: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []fileRow
	for rows.Next() {
		var f fileRow
		if err := rows.Scan(&f.id, &f.name, &f.ext, &f.path, &f.pid, &f.createTime,
			&f.score, &f.note, &f.origin); err != nil {
			return nil, fmt.Errorf("billfish: scan file: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// folderPaths returns the single folder membership (root→leaf) for a file's pid,
// or nil when the file sits at the root (pid 0).
func (c *catalog) folderPaths(pid int64) []importers.NamePath {
	if p := pathFor(pid, c.folderName, c.folderParent); len(p) > 0 {
		return []importers.NamePath{p}
	}
	return nil
}

// tagPaths returns each of a file's tags as a root→leaf path.
func (c *catalog) tagPaths(fileID int64) []importers.NamePath {
	ids := c.fileTags[fileID]
	out := make([]importers.NamePath, 0, len(ids))
	for _, id := range ids {
		if p := pathFor(id, c.tagName, c.tagParent); len(p) > 0 {
			out = append(out, p)
		}
	}
	return out
}

// pathFor walks the parent chain from id to the root, returning the root→leaf
// name segments. A pid of 0 (or a broken/cyclic chain) stops the walk.
func pathFor(id int64, name map[int64]string, parent map[int64]int64) importers.NamePath {
	var segs importers.NamePath
	seen := map[int64]bool{}
	for id != 0 && !seen[id] {
		seen[id] = true
		nm, ok := name[id]
		if !ok {
			break
		}
		segs = append(importers.NamePath{nm}, segs...)
		id = parent[id]
	}
	return segs
}
