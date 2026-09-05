package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

func TestSQLite_UpsertAndList(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatalf("ensure owner: %v", err)
	}

	id, err := domain.NewAssetID("asset-1")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	asset := domain.Asset{
		ID: id, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: "a/b.png", Name: "b.png", Ext: "png", Size: 123,
		Hash: "deadbeef", ThumbPath: "/thumbs/1.jpg", Width: 800, Height: 600,
		CreatedAt: now, IndexedAt: now,
	}
	if err := st.UpsertAsset(ctx, asset); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := st.ListAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Name != "b.png" || got[0].Width != 800 || got[0].Hash != "deadbeef" {
		t.Fatalf("round-trip mismatch: %+v", got[0])
	}

	// Upserting the same (owner, provider, path) updates in place, no duplicate.
	asset.Size = 999
	if err := st.UpsertAsset(ctx, asset); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got, err = st.ListAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("after re-upsert len = %d, want 1", len(got))
	}
	if got[0].Size != 999 {
		t.Fatalf("size = %d, want 999 (updated)", got[0].Size)
	}
}

func mkAsset(t *testing.T, owner domain.OwnerID, id, name string) domain.Asset {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	return domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: id + ".png", Name: name, Ext: "png", Size: 1,
		Hash: id, Width: 1, Height: 1, CreatedAt: now, IndexedAt: now,
	}
}

func TestSQLite_SearchAssets(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "search.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatalf("ensure owner: %v", err)
	}
	for _, a := range []domain.Asset{
		mkAsset(t, owner, "asset-1", "sunset over the sea"),
		mkAsset(t, owner, "asset-2", "sunset beach"),
		mkAsset(t, owner, "asset-3", "mountain peak"),
	} {
		if err := st.UpsertAsset(ctx, a); err != nil {
			t.Fatalf("upsert %s: %v", a.ID, err)
		}
	}

	find := func(q string) []domain.Asset {
		t.Helper()
		got, err := st.SearchAssets(ctx, owner, q, 50, 0)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		return got
	}

	// Match by name across multiple assets, ordered by bm25 relevance:
	// the shorter document ("sunset beach") ranks ahead of the longer one.
	got := find("sunset")
	if len(got) != 2 {
		t.Fatalf("search sunset = %d results, want 2", len(got))
	}
	if got[0].Name != "sunset beach" {
		t.Errorf("relevance order: first = %q, want %q", got[0].Name, "sunset beach")
	}
	if n := len(find("mountain")); n != 1 {
		t.Errorf("search mountain = %d, want 1", n)
	}
	if n := len(find("nonexistent")); n != 0 {
		t.Errorf("search nonexistent = %d, want 0", n)
	}

	// Update: renaming asset-3 removes the old term and adds the new one.
	if err := st.UpsertAsset(ctx, mkAsset(t, owner, "asset-3", "sunset valley")); err != nil {
		t.Fatalf("update: %v", err)
	}
	if n := len(find("mountain")); n != 0 {
		t.Errorf("after rename, search mountain = %d, want 0", n)
	}
	if n := len(find("sunset")); n != 3 {
		t.Errorf("after rename, search sunset = %d, want 3", n)
	}

	// Delete: removing an asset row removes it from the FTS index. There is no
	// DeleteAsset API yet, so issue the DELETE via a second raw connection to
	// exercise the AFTER DELETE trigger.
	raw, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx, "DELETE FROM assets WHERE id = ?", "asset-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n := len(find("sunset")); n != 2 {
		t.Errorf("after delete, search sunset = %d, want 2", n)
	}
}

// legacyAssetsSchema mirrors a database created before assets_fts existed: the
// current assets/users columns but no assets_fts table or triggers, to simulate
// an in-place upgrade onto the FTS index.
const legacyAssetsSchema = `
CREATE TABLE users (id TEXT PRIMARY KEY, created_at INTEGER NOT NULL);
CREATE TABLE assets (
    id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, kind TEXT NOT NULL,
    provider TEXT NOT NULL, storage_path TEXT NOT NULL, name TEXT NOT NULL,
    ext TEXT NOT NULL, size INTEGER NOT NULL, hash TEXT NOT NULL,
    thumb_path TEXT NOT NULL DEFAULT '', width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL,
    indexed_at INTEGER NOT NULL, deleted_at INTEGER, rating INTEGER NOT NULL DEFAULT 0,
    color TEXT NOT NULL DEFAULT '', display_name TEXT NOT NULL DEFAULT '',
    folder_id TEXT NOT NULL DEFAULT '');
INSERT INTO users(id, created_at) VALUES('tester', 0);
INSERT INTO assets(id, owner_id, kind, provider, storage_path, name, ext, size, hash, created_at, indexed_at)
VALUES('legacy-1', 'tester', 'image', 'local', 'legacy-1.png', 'legacy sunset', 'png', 1, 'h', 0, 0);`

// TestSQLite_SearchAssets_LegacyBackfill verifies that a database created before
// assets_fts existed is backfilled on Open, so pre-existing rows are searchable
// and a subsequent upsert does not corrupt the DB via the UPDATE trigger's
// contentless delete against an unindexed row.
func TestSQLite_SearchAssets_LegacyBackfill(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, legacyAssetsSchema); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(ctx, dbPath) // adds assets_fts + triggers + backfill
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	find := func(q string) []domain.Asset {
		t.Helper()
		got, err := st.SearchAssets(ctx, owner, q, 50, 0)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		return got
	}

	// The legacy row is searchable thanks to the backfill.
	if n := len(find("sunset")); n != 1 {
		t.Fatalf("after backfill, search sunset = %d, want 1", n)
	}
	// Re-upsert (same storage_path) fires the UPDATE trigger; without the
	// backfill this corrupts the DB. With it, the rename syncs cleanly.
	if err := st.UpsertAsset(ctx, mkAsset(t, owner, "legacy-1", "legacy moon")); err != nil {
		t.Fatalf("re-upsert legacy row: %v", err)
	}
	if n := len(find("sunset")); n != 0 {
		t.Errorf("after rename, search sunset = %d, want 0", n)
	}
	if n := len(find("moon")); n != 1 {
		t.Errorf("after rename, search moon = %d, want 1", n)
	}
}

// TestSQLite_SearchAssets_Tags verifies that tags attached to assets are indexed
// in assets_fts and searchable via the "tags" FTS5 column.
func TestSQLite_SearchAssets_Tags(t *testing.T) {
	ctx, st, owner, _ := openAt(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "photos/sunset.jpg")
	a2 := seedAsset(t, ctx, st, owner, "a2", "photos/mountain.jpg")

	// Create tags once, then attach to assets (UNIQUE on owner_id+name).
	tidLand, err := domain.NewTagID("t-landscape")
	if err != nil {
		t.Fatal(err)
	}
	tidSun, err := domain.NewTagID("t-sunset")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTag(ctx, domain.Tag{ID: tidLand, Owner: owner, Name: "landscape"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTag(ctx, domain.Tag{ID: tidSun, Owner: owner, Name: "sunset"}); err != nil {
		t.Fatal(err)
	}
	// a1 gets both tags; a2 gets only landscape.
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{a1}, []domain.TagID{tidLand, tidSun}); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{a2}, []domain.TagID{tidLand}); err != nil {
		t.Fatal(err)
	}

	find := func(q string) []domain.Asset {
		t.Helper()
		got, err := st.SearchAssets(ctx, owner, q, 50, 0)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		return got
	}

	// Field-qualified tag search.
	if n := len(find(`tags : "landscape"`)); n != 2 {
		t.Errorf("search tags:landscape = %d, want 2", n)
	}
	if n := len(find(`tags : "sunset"`)); n != 1 {
		t.Errorf("search tags:sunset = %d, want 1", n)
	}
	// Unqualified search matches tags too.
	if n := len(find(`"landscape"`)); n != 2 {
		t.Errorf("search landscape (unqualified) = %d, want 2", n)
	}
}

// TestSQLite_SearchAssets_Description verifies that caption annotations are
// indexed in assets_fts.description and searchable via the "description" column.
func TestSQLite_SearchAssets_Description(t *testing.T) {
	ctx, st, owner, _ := openAt(t)
	aid := seedAsset(t, ctx, st, owner, "a1", "photos/cat.jpg")

	// Persist AI caption — this writes to annotations(layer='ai', key='caption').
	res := domain.NewAITagResult(nil, "a fluffy cat sleeping on a sunny windowsill", "test-model")
	if err := st.PersistAITagResult(ctx, owner, aid, res); err != nil {
		t.Fatalf("persist caption: %v", err)
	}

	find := func(q string) []domain.Asset {
		t.Helper()
		got, err := st.SearchAssets(ctx, owner, q, 50, 0)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		return got
	}

	// Field-qualified description search.
	if n := len(find(`description : "fluffy"`)); n != 1 {
		t.Errorf("search desc:fluffy = %d, want 1", n)
	}
	if n := len(find(`description : "windowsill"`)); n != 1 {
		t.Errorf("search desc:windowsill = %d, want 1", n)
	}
	// Unqualified search matches description too.
	if n := len(find(`"sunny"`)); n != 1 {
		t.Errorf("search sunny (unqualified) = %d, want 1", n)
	}
	// No match.
	if n := len(find(`description : "dog"`)); n != 0 {
		t.Errorf("search desc:dog = %d, want 0", n)
	}
}

// TestSQLite_SearchAssets_TagRemoval verifies that removing a tag from an asset
// updates the FTS index so the term is no longer searchable via tags.
func TestSQLite_SearchAssets_TagRemoval(t *testing.T) {
	ctx, st, owner, _ := openAt(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "photos/sunset.jpg")

	tid, err := domain.NewTagID("t-temp")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTag(ctx, domain.Tag{ID: tid, Owner: owner, Name: "temporary"}); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{a1}, []domain.TagID{tid}); err != nil {
		t.Fatal(err)
	}

	find := func(q string) []domain.Asset {
		t.Helper()
		got, err := st.SearchAssets(ctx, owner, q, 50, 0)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		return got
	}

	// Tag is searchable after add.
	if n := len(find(`tags : "temporary"`)); n != 1 {
		t.Fatalf("after add, search tags:temporary = %d, want 1", n)
	}
	// Remove the tag.
	if err := st.BatchRemoveTags(ctx, owner, []domain.AssetID{a1}, tid); err != nil {
		t.Fatal(err)
	}
	// Tag is no longer searchable.
	if n := len(find(`tags : "temporary"`)); n != 0 {
		t.Errorf("after remove, search tags:temporary = %d, want 0", n)
	}
}

// TestSQLite_SearchAssets_InvalidQuery verifies malformed FTS5 MATCH expressions
// are reported as domain.ErrInvalidQuery (mapped to HTTP 400) rather than a
// generic server error. The parser never emits these; this guards the store's
// defense-in-depth mapping directly.
func TestSQLite_SearchAssets_InvalidQuery(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "invalid.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`"a" AND`, `*`} {
		if _, err := st.SearchAssets(ctx, owner, q, 10, 0); !errors.Is(err, domain.ErrInvalidQuery) {
			t.Errorf("SearchAssets(%q) err = %v, want ErrInvalidQuery", q, err)
		}
	}
}
