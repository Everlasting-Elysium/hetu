package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// openAt opens a store at an explicit path so tests can also inspect rows with
// raw SQL (the store exposes no source/layer read methods).
func openAt(t *testing.T) (context.Context, *store.SQLite, domain.OwnerID, string) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")
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
	return ctx, st, owner, dbPath
}

// seedManualTag creates a manual tag named name and links it to assetID with
// source='manual', mirroring what the tags plugin does for a user tag.
func seedManualTag(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, assetID domain.AssetID, tagID, name string) {
	t.Helper()
	id, err := domain.NewTagID(tagID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTag(ctx, domain.Tag{ID: id, Owner: owner, Name: name}); err != nil {
		t.Fatalf("create manual tag: %v", err)
	}
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{assetID}, []domain.TagID{id}); err != nil {
		t.Fatalf("link manual tag: %v", err)
	}
}

// seedManualCaption inserts a manual-layer caption annotation directly.
func seedManualCaption(t *testing.T, ctx context.Context, dbPath, assetID, jsonValue string) {
	t.Helper()
	raw := mustRaw(t, dbPath)
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO annotations (asset_id, layer, "key", value, model, created_at)
		 VALUES (?, 'manual', 'caption', ?, '', 0)`, assetID, jsonValue); err != nil {
		t.Fatalf("seed manual caption: %v", err)
	}
}

func mustRaw(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// tagSource returns the asset_tags.source for the (asset, tag-name) link.
func tagSource(t *testing.T, ctx context.Context, dbPath, assetID, tagName string) (string, bool) {
	t.Helper()
	raw := mustRaw(t, dbPath)
	defer func() { _ = raw.Close() }()
	var source string
	err := raw.QueryRowContext(ctx,
		`SELECT at.source FROM asset_tags at JOIN tags t ON t.id = at.tag_id
		 WHERE at.asset_id = ? AND t.name = ?`, assetID, tagName).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read tag source: %v", err)
	}
	return source, true
}

// annotation returns the value+model of an annotation row (ok=false if absent).
func annotation(t *testing.T, ctx context.Context, dbPath, assetID, layer, key string) (value, model string, ok bool) {
	t.Helper()
	raw := mustRaw(t, dbPath)
	defer func() { _ = raw.Close() }()
	err := raw.QueryRowContext(ctx,
		`SELECT value, model FROM annotations WHERE asset_id=? AND layer=? AND "key"=?`,
		assetID, layer, key).Scan(&value, &model)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false
	}
	if err != nil {
		t.Fatalf("read annotation: %v", err)
	}
	return value, model, true
}

func countTags(t *testing.T, ctx context.Context, dbPath, assetID string) int {
	t.Helper()
	raw := mustRaw(t, dbPath)
	defer func() { _ = raw.Close() }()
	var n int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM asset_tags WHERE asset_id=?`, assetID).Scan(&n); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	return n
}

// TestSQLite_PersistAITagResult_landsAndPreservesManual proves the ai layer is
// written (asset_tags source='ai', annotations layer='ai' with model) while a
// manual tag link and a manual caption on the same asset are left untouched.
func TestSQLite_PersistAITagResult_landsAndPreservesManual(t *testing.T) {
	ctx, st, owner, dbPath := openAt(t)
	aid := seedAsset(t, ctx, st, owner, "photo-1", "photos/cat.jpg")
	seedManualTag(t, ctx, st, owner, aid, "man-cat", "cat")
	seedManualCaption(t, ctx, dbPath, "photo-1", `"human wrote this"`)

	res := domain.NewAITagResult([]string{"cat", "animal"}, "a cat on a mat", "stub-v1")
	if err := st.PersistAITagResult(ctx, owner, aid, res); err != nil {
		t.Fatalf("PersistAITagResult: %v", err)
	}

	// The pre-existing manual link stays manual (AI never overwrites it).
	if src, ok := tagSource(t, ctx, dbPath, "photo-1", "cat"); !ok || src != "manual" {
		t.Errorf("cat link source = %q (ok=%v), want manual", src, ok)
	}
	// The new AI-only tag lands with source='ai'.
	if src, ok := tagSource(t, ctx, dbPath, "photo-1", "animal"); !ok || src != "ai" {
		t.Errorf("animal link source = %q (ok=%v), want ai", src, ok)
	}
	// The ai caption lands in the ai layer with its model.
	if v, m, ok := annotation(t, ctx, dbPath, "photo-1", "ai", "caption"); !ok || v != `"a cat on a mat"` || m != "stub-v1" {
		t.Errorf("ai caption = (%q, %q, ok=%v), want (\"a cat on a mat\", stub-v1)", v, m, ok)
	}
	// The manual caption is a separate row, untouched.
	if v, _, ok := annotation(t, ctx, dbPath, "photo-1", "manual", "caption"); !ok || v != `"human wrote this"` {
		t.Errorf("manual caption = (%q, ok=%v), want \"human wrote this\"", v, ok)
	}
}

// TestSQLite_ClearAILayer_removesOnlyAI proves clearing wipes ai-sourced tags
// and ai-layer annotations while manual rows survive.
func TestSQLite_ClearAILayer_removesOnlyAI(t *testing.T) {
	ctx, st, owner, dbPath := openAt(t)
	aid := seedAsset(t, ctx, st, owner, "photo-1", "photos/cat.jpg")
	seedManualTag(t, ctx, st, owner, aid, "man-cat", "cat")
	seedManualCaption(t, ctx, dbPath, "photo-1", `"human wrote this"`)
	res := domain.NewAITagResult([]string{"cat", "animal"}, "ai caption", "stub-v1")
	if err := st.PersistAITagResult(ctx, owner, aid, res); err != nil {
		t.Fatalf("PersistAITagResult: %v", err)
	}

	if err := st.ClearAILayer(ctx, owner); err != nil {
		t.Fatalf("ClearAILayer: %v", err)
	}

	// AI-only tag link and ai caption are gone.
	if _, ok := tagSource(t, ctx, dbPath, "photo-1", "animal"); ok {
		t.Error("ai tag 'animal' link should be cleared")
	}
	if _, _, ok := annotation(t, ctx, dbPath, "photo-1", "ai", "caption"); ok {
		t.Error("ai caption should be cleared")
	}
	// Manual link and manual caption remain intact.
	if src, ok := tagSource(t, ctx, dbPath, "photo-1", "cat"); !ok || src != "manual" {
		t.Errorf("manual cat link should survive clear, got %q (ok=%v)", src, ok)
	}
	if v, _, ok := annotation(t, ctx, dbPath, "photo-1", "manual", "caption"); !ok || v != `"human wrote this"` {
		t.Errorf("manual caption should survive clear, got %q (ok=%v)", v, ok)
	}
}

// TestSQLite_PersistAITagResult_idempotent proves re-running does not duplicate
// tag links and upserts the caption in place (model upgrade re-run).
func TestSQLite_PersistAITagResult_idempotent(t *testing.T) {
	ctx, st, owner, dbPath := openAt(t)
	aid := seedAsset(t, ctx, st, owner, "photo-1", "photos/cat.jpg")

	first := domain.NewAITagResult([]string{"cat", "animal"}, "old caption", "stub-v1")
	if err := st.PersistAITagResult(ctx, owner, aid, first); err != nil {
		t.Fatalf("first persist: %v", err)
	}
	// Re-run with the same tags but an upgraded model + caption.
	second := domain.NewAITagResult([]string{"cat", "animal"}, "new caption", "stub-v2")
	if err := st.PersistAITagResult(ctx, owner, aid, second); err != nil {
		t.Fatalf("second persist: %v", err)
	}

	if n := countTags(t, ctx, dbPath, "photo-1"); n != 2 {
		t.Errorf("asset_tags count = %d, want 2 (no duplicates on re-run)", n)
	}
	if v, m, ok := annotation(t, ctx, dbPath, "photo-1", "ai", "caption"); !ok || v != `"new caption"` || m != "stub-v2" {
		t.Errorf("ai caption = (%q, %q, ok=%v), want (\"new caption\", stub-v2)", v, m, ok)
	}
}
