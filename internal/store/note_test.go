package store_test

import (
	"context"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/search"
)

// searchIDs runs an FTS query and returns the matched asset id set.
func searchIDs(t *testing.T, ctx context.Context, st interface {
	SearchAssets(context.Context, domain.OwnerID, string, int, int) ([]domain.Asset, error)
}, owner domain.OwnerID, q string) map[string]bool {
	t.Helper()
	fts, err := search.Parse(q)
	if err != nil {
		t.Fatalf("parse query %q: %v", q, err)
	}
	assets, err := st.SearchAssets(ctx, owner, fts, 50, 0)
	if err != nil {
		t.Fatalf("search %q: %v", q, err)
	}
	ids := make(map[string]bool, len(assets))
	for _, a := range assets {
		ids[a.ID.String()] = true
	}
	return ids
}

// TestSQLite_UpsertManualCaption_writesAndIndexes proves a manual caption lands
// in the manual layer (JSON-encoded) and is immediately full-text searchable.
func TestSQLite_UpsertManualCaption_writesAndIndexes(t *testing.T) {
	ctx, st, owner, dbPath := openAt(t)
	aid := seedAsset(t, ctx, st, owner, "photo-1", "photos/cat.jpg")

	if err := st.UpsertManualCaption(ctx, owner, aid, "a fluffy kitten"); err != nil {
		t.Fatalf("UpsertManualCaption: %v", err)
	}

	// Stored JSON-encoded in the manual layer.
	if v, m, ok := annotation(t, ctx, dbPath, "photo-1", "manual", "caption"); !ok || v != `"a fluffy kitten"` || m != "" {
		t.Errorf("manual caption = (%q, %q, ok=%v), want (\"a fluffy kitten\", \"\")", v, m, ok)
	}
	// Searchable via FTS description column.
	if ids := searchIDs(t, ctx, st, owner, "kitten"); !ids["photo-1"] {
		t.Error("manual caption text should be full-text searchable")
	}
}

// TestSQLite_UpsertManualCaption_replaces proves a second upsert overwrites the
// prior manual caption in place (no duplicate rows) and re-indexes FTS.
func TestSQLite_UpsertManualCaption_replaces(t *testing.T) {
	ctx, st, owner, dbPath := openAt(t)
	aid := seedAsset(t, ctx, st, owner, "photo-1", "photos/cat.jpg")

	if err := st.UpsertManualCaption(ctx, owner, aid, "first draft"); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := st.UpsertManualCaption(ctx, owner, aid, "second draft"); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if v, _, ok := annotation(t, ctx, dbPath, "photo-1", "manual", "caption"); !ok || v != `"second draft"` {
		t.Errorf("manual caption = (%q, ok=%v), want \"second draft\"", v, ok)
	}
	// Old text no longer hits; new text does.
	if ids := searchIDs(t, ctx, st, owner, "first"); ids["photo-1"] {
		t.Error("old caption text should no longer be searchable")
	}
	if ids := searchIDs(t, ctx, st, owner, "second"); !ids["photo-1"] {
		t.Error("new caption text should be searchable")
	}
}

// TestSQLite_DeleteManualCaption_clearsFTS proves deleting the manual caption
// removes the row and clears the FTS description.
func TestSQLite_DeleteManualCaption_clearsFTS(t *testing.T) {
	ctx, st, owner, dbPath := openAt(t)
	aid := seedAsset(t, ctx, st, owner, "photo-1", "photos/cat.jpg")
	if err := st.UpsertManualCaption(ctx, owner, aid, "temporary note"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := st.DeleteManualCaption(ctx, owner, aid); err != nil {
		t.Fatalf("DeleteManualCaption: %v", err)
	}

	if _, _, ok := annotation(t, ctx, dbPath, "photo-1", "manual", "caption"); ok {
		t.Error("manual caption row should be deleted")
	}
	if ids := searchIDs(t, ctx, st, owner, "temporary"); ids["photo-1"] {
		t.Error("deleted caption text should no longer be searchable")
	}
}

// TestSQLite_ManualCaption_priorityOverAI proves the FTS description prefers the
// manual caption over an AI caption, and reverts to AI after manual deletion.
func TestSQLite_ManualCaption_priorityOverAI(t *testing.T) {
	ctx, st, owner, _ := openAt(t)
	aid := seedAsset(t, ctx, st, owner, "photo-1", "photos/cat.jpg")

	// AI caption first.
	ai := domain.NewAITagResult(nil, "aicaption robot", "stub-v1")
	if err := st.PersistAITagResult(ctx, owner, aid, ai); err != nil {
		t.Fatalf("persist ai: %v", err)
	}
	if ids := searchIDs(t, ctx, st, owner, "aicaption"); !ids["photo-1"] {
		t.Error("ai caption should be searchable before manual is set")
	}

	// Manual caption takes priority in the description column.
	if err := st.UpsertManualCaption(ctx, owner, aid, "humancaption person"); err != nil {
		t.Fatalf("upsert manual: %v", err)
	}
	if ids := searchIDs(t, ctx, st, owner, "humancaption"); !ids["photo-1"] {
		t.Error("manual caption should be searchable (highest priority)")
	}
	if ids := searchIDs(t, ctx, st, owner, "aicaption"); ids["photo-1"] {
		t.Error("ai caption should be shadowed by manual in FTS description")
	}

	// Deleting the manual caption reverts the description to the AI caption.
	if err := st.DeleteManualCaption(ctx, owner, aid); err != nil {
		t.Fatalf("delete manual: %v", err)
	}
	if ids := searchIDs(t, ctx, st, owner, "aicaption"); !ids["photo-1"] {
		t.Error("ai caption should be searchable again after manual delete")
	}
}

// TestSQLite_ListManualCaptions_ownerScopedAndBatched proves the batch read
// returns only the requested owner's manual captions, keyed by asset id.
func TestSQLite_ListManualCaptions_ownerScopedAndBatched(t *testing.T) {
	ctx, st, owner, _ := openAt(t)
	a1 := seedAsset(t, ctx, st, owner, "photo-1", "photos/a.jpg")
	a2 := seedAsset(t, ctx, st, owner, "photo-2", "photos/b.jpg")
	a3 := seedAsset(t, ctx, st, owner, "photo-3", "photos/c.jpg")
	if err := st.UpsertManualCaption(ctx, owner, a1, "note one"); err != nil {
		t.Fatalf("upsert a1: %v", err)
	}
	if err := st.UpsertManualCaption(ctx, owner, a2, "note two"); err != nil {
		t.Fatalf("upsert a2: %v", err)
	}
	// a3 has no note.

	notes, err := st.ListManualCaptions(ctx, owner, []domain.AssetID{a1, a2, a3})
	if err != nil {
		t.Fatalf("ListManualCaptions: %v", err)
	}
	if len(notes) != 2 {
		t.Errorf("notes count = %d, want 2", len(notes))
	}
	if notes["photo-1"] != "note one" {
		t.Errorf("photo-1 note = %q, want %q", notes["photo-1"], "note one")
	}
	if notes["photo-2"] != "note two" {
		t.Errorf("photo-2 note = %q, want %q", notes["photo-2"], "note two")
	}
	if _, ok := notes["photo-3"]; ok {
		t.Error("photo-3 should have no note entry")
	}
}

// TestSQLite_ListManualCaptions_otherOwnerExcluded proves an asset id owned by a
// different owner is never returned even if its id is requested.
func TestSQLite_ListManualCaptions_otherOwnerExcluded(t *testing.T) {
	ctx, st, owner, _ := openAt(t)
	a1 := seedAsset(t, ctx, st, owner, "photo-1", "photos/a.jpg")
	if err := st.UpsertManualCaption(ctx, owner, a1, "mine"); err != nil {
		t.Fatalf("upsert a1: %v", err)
	}

	other, err := domain.NewOwnerID("intruder")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, other); err != nil {
		t.Fatalf("ensure other owner: %v", err)
	}

	// The intruder asks for photo-1 (not theirs): scoping must exclude it.
	notes, err := st.ListManualCaptions(ctx, other, []domain.AssetID{a1})
	if err != nil {
		t.Fatalf("ListManualCaptions: %v", err)
	}
	if _, ok := notes["photo-1"]; ok {
		t.Error("other owner must not see photo-1's manual caption")
	}
}

// TestSQLite_ManualCaption_missingAsset proves operations on an unknown asset
// return domain.ErrNotFound.
func TestSQLite_ManualCaption_missingAsset(t *testing.T) {
	ctx, st, owner, _ := openAt(t)
	ghost, err := domain.NewAssetID("ghost")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertManualCaption(ctx, owner, ghost, "hi"); err == nil {
		t.Error("upsert on missing asset should error")
	}
	if err := st.DeleteManualCaption(ctx, owner, ghost); err == nil {
		t.Error("delete on missing asset should error")
	}
}
