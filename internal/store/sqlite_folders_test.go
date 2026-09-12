package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

func mkFolder(t *testing.T, owner domain.OwnerID, id, name, path string) domain.Folder {
	t.Helper()
	fid, err := domain.NewFolderID(id)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Folder{ID: fid, Owner: owner, Name: name, Path: path}
}

func TestSQLite_FolderCRUD(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	photos := mkFolder(t, owner, "f1", "Photos", "/Photos")
	videos := mkFolder(t, owner, "f2", "Videos", "/Videos")
	if err := st.CreateFolder(ctx, photos); err != nil {
		t.Fatalf("create photos: %v", err)
	}
	if err := st.CreateFolder(ctx, videos); err != nil {
		t.Fatalf("create videos: %v", err)
	}

	folders, err := st.ListFolders(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 {
		t.Fatalf("folders = %d, want 2", len(folders))
	}
	// Ordered by path.
	if folders[0].Path != "/Photos" || folders[1].Path != "/Videos" {
		t.Fatalf("order = %q,%q, want /Photos,/Videos", folders[0].Path, folders[1].Path)
	}

	if err := st.DeleteFolder(ctx, owner, photos.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	folders, err = st.ListFolders(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || folders[0].Name != "Videos" {
		t.Fatalf("after delete = %+v, want [Videos]", folders)
	}
}

// TestSQLite_FolderCoverColor covers the explicit cover override + color label
// round-trip through GetFolder (raw) and UpdateFolderCover, plus the IDOR/dangling
// cover guard.
func TestSQLite_FolderCoverColor(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	f := mkFolder(t, owner, "f1", "Photos", "/Photos")
	if err := st.CreateFolder(ctx, f); err != nil {
		t.Fatalf("create: %v", err)
	}
	aid := seedAsset(t, ctx, st, owner, "a1", "1.png")

	if err := st.UpdateFolderCover(ctx, owner, f.ID, aid.String(), "#e5484d"); err != nil {
		t.Fatalf("set cover/color: %v", err)
	}
	got, err := st.GetFolder(ctx, owner, f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cover != "a1" || got.Color != "#e5484d" {
		t.Fatalf("get folder = cover %q color %q, want a1 #e5484d", got.Cover, got.Color)
	}

	// A dangling cover id is rejected without mutating the row.
	if err := st.UpdateFolderCover(ctx, owner, f.ID, "ghost", ""); err == nil {
		t.Fatal("expected error for dangling cover, got nil")
	}
	if got, _ := st.GetFolder(ctx, owner, f.ID); got.Cover != "a1" {
		t.Fatalf("cover after rejected update = %q, want a1 (unchanged)", got.Cover)
	}

	// Clearing the override is allowed and empties the stored cover.
	if err := st.UpdateFolderCover(ctx, owner, f.ID, "", "#46a758"); err != nil {
		t.Fatalf("clear cover: %v", err)
	}
	if got, _ := st.GetFolder(ctx, owner, f.ID); got.Cover != "" || got.Color != "#46a758" {
		t.Fatalf("after clear = cover %q color %q, want empty #46a758", got.Cover, got.Color)
	}
}

// TestSQLite_FolderCoverFallback proves ListFolders resolves the effective cover
// to the folder's earliest-indexed live asset when no override is set.
func TestSQLite_FolderCoverFallback(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	f := mkFolder(t, owner, "f1", "Photos", "/Photos")
	if err := st.CreateFolder(ctx, f); err != nil {
		t.Fatalf("create: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	seedFolderAssetAt(t, ctx, st, owner, "late", "f1", now)
	seedFolderAssetAt(t, ctx, st, owner, "early", "f1", now.Add(-time.Hour))

	folders, err := st.ListFolders(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if folders[0].Cover != "early" {
		t.Fatalf("fallback cover = %q, want early", folders[0].Cover)
	}
}

// seedFolderAssetAt upserts a live image asset in folderID indexed at ts.
func seedFolderAssetAt(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id, folderID string, ts time.Time) {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: id + ".png", Name: id + ".png", Ext: "png", Size: 1, Hash: "h-" + id,
		CreatedAt: ts, IndexedAt: ts, FolderID: folderID,
	}); err != nil {
		t.Fatalf("seed folder asset %s: %v", id, err)
	}
}

// TestSQLite_FolderColumnMigration proves a database whose folders table predates
// the cover/color columns gains them via ALTER TABLE on Open, preserving rows.
func TestSQLite_FolderColumnMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Create a pre-cover/color folders table and seed a row directly.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE folders (
		id TEXT PRIMARY KEY, owner_id TEXT NOT NULL,
		parent_id TEXT NOT NULL DEFAULT '', name TEXT NOT NULL, path TEXT NOT NULL)`); err != nil {
		t.Fatalf("create legacy folders: %v", err)
	}
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO folders (id, owner_id, name, path) VALUES ('f1', 'tester', 'Legacy', '/Legacy')`); err != nil {
		t.Fatalf("seed legacy folder: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	// Opening the store must migrate the columns in and keep the row readable.
	st, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	folders, err := st.ListFolders(ctx, owner)
	if err != nil {
		t.Fatalf("list migrated folders: %v", err)
	}
	if len(folders) != 1 || folders[0].Name != "Legacy" || folders[0].Cover != "" || folders[0].Color != "" {
		t.Fatalf("migrated folder = %+v, want Legacy with empty cover/color", folders)
	}
}
