package store_test

import (
	"errors"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func TestSQLite_CollectionItemsOrderIdempotent(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	c := mkCollection(t, owner, "c1", "C", "")
	if err := st.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")
	a3 := seedAsset(t, ctx, st, owner, "a3", "3.png")
	addItem(t, ctx, st, owner, c.ID, a1)
	addItem(t, ctx, st, owner, c.ID, a2)
	addItem(t, ctx, st, owner, c.ID, a3)

	items, err := st.ListCollectionItems(ctx, owner, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	// Appended in add order with ascending ord.
	if items[0].AssetID.String() != "a1" || items[0].Ord != 0 ||
		items[1].AssetID.String() != "a2" || items[1].Ord != 1 ||
		items[2].AssetID.String() != "a3" || items[2].Ord != 2 {
		t.Fatalf("items = %+v, want a1/0 a2/1 a3/2", items)
	}
	// Enrichment resolves the asset's kind/name via the JOIN.
	if items[0].AssetKind != string(domain.KindImage) || items[0].AssetName != "1.png" {
		t.Fatalf("item0 enrich = %+v", items[0])
	}

	// Re-adding an existing member updates its ord (moves to end) instead of
	// erroring or creating a duplicate row.
	addItem(t, ctx, st, owner, c.ID, a1)
	items, _ = st.ListCollectionItems(ctx, owner, c.ID)
	if len(items) != 3 {
		t.Fatalf("after re-add items = %d, want 3 (no duplicate)", len(items))
	}
	if items[2].AssetID.String() != "a1" {
		t.Fatalf("after re-add last = %s, want a1 (moved to end)", items[2].AssetID)
	}
}

func TestSQLite_RemoveCollectionItem(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	c := mkCollection(t, owner, "c1", "C", "")
	if err := st.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")
	addItem(t, ctx, st, owner, c.ID, a1)
	addItem(t, ctx, st, owner, c.ID, a2)
	if err := st.RemoveCollectionItem(ctx, owner, c.ID, a1); err != nil {
		t.Fatalf("remove: %v", err)
	}
	items, _ := st.ListCollectionItems(ctx, owner, c.ID)
	if len(items) != 1 || items[0].AssetID.String() != "a2" {
		t.Fatalf("after remove = %+v, want [a2]", items)
	}
}

func TestSQLite_ReorderCollectionItems(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	c := mkCollection(t, owner, "c1", "C", "")
	if err := st.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")
	a3 := seedAsset(t, ctx, st, owner, "a3", "3.png")
	addItem(t, ctx, st, owner, c.ID, a1)
	addItem(t, ctx, st, owner, c.ID, a2)
	addItem(t, ctx, st, owner, c.ID, a3)

	// Normal reorder: a3, a1, a2 with ords 0,1,2.
	if err := st.ReorderCollectionItems(ctx, owner, c.ID, []domain.AssetID{a3, a1, a2}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	items, _ := st.ListCollectionItems(ctx, owner, c.ID)
	if items[0].AssetID.String() != "a3" || items[1].AssetID.String() != "a1" || items[2].AssetID.String() != "a2" {
		t.Fatalf("reordered = %+v, want [a3 a1 a2]", items)
	}
	if items[0].Ord != 0 || items[1].Ord != 1 || items[2].Ord != 2 {
		t.Fatalf("ords = %d,%d,%d, want 0,1,2", items[0].Ord, items[1].Ord, items[2].Ord)
	}

	// Mismatch by count (fewer than the members).
	if err := st.ReorderCollectionItems(ctx, owner, c.ID, []domain.AssetID{a1, a2}); !errors.Is(err, domain.ErrCollectionItemsMismatch) {
		t.Fatalf("count mismatch err = %v, want ErrCollectionItemsMismatch", err)
	}

	// Mismatch by membership: a non-member swapped in at the same count.
	a4 := seedAsset(t, ctx, st, owner, "a4", "4.png")
	if err := st.ReorderCollectionItems(ctx, owner, c.ID, []domain.AssetID{a1, a2, a4}); !errors.Is(err, domain.ErrCollectionItemsMismatch) {
		t.Fatalf("member mismatch err = %v, want ErrCollectionItemsMismatch", err)
	}
}

// TestSQLite_AddCollectionItem_otherOwnerAssetRejected proves an asset id owned
// by a different owner cannot be attached to one's own collection (IDOR guard —
// otherwise its kind/name/thumb would leak via the enriched item list).
func TestSQLite_AddCollectionItem_otherOwnerAssetRejected(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	c := mkCollection(t, owner, "c1", "C", "")
	if err := st.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}

	other, err := domain.NewOwnerID("intruder")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, other); err != nil {
		t.Fatalf("ensure other owner: %v", err)
	}
	theirs := seedAsset(t, ctx, st, other, "theirs", "secret.png")

	if err := st.AddCollectionItem(ctx, owner, c.ID, theirs); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("add other-owner asset err = %v, want ErrNotFound", err)
	}
	items, _ := st.ListCollectionItems(ctx, owner, c.ID)
	if len(items) != 0 {
		t.Fatalf("items after rejected add = %+v, want none", items)
	}
}

// TestSQLite_RemoveCollectionItemClearsStaleCover proves the "cover is empty or
// a current member" invariant survives removal: removing the explicit-cover
// member must clear the override rather than leaving it dangling.
func TestSQLite_RemoveCollectionItemClearsStaleCover(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	c := mkCollection(t, owner, "c1", "C", "")
	if err := st.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")
	addItem(t, ctx, st, owner, c.ID, a1)
	addItem(t, ctx, st, owner, c.ID, a2)
	if err := st.UpdateCollection(ctx, owner, c.ID, "C", "", "a1"); err != nil {
		t.Fatalf("set cover: %v", err)
	}

	if err := st.RemoveCollectionItem(ctx, owner, c.ID, a1); err != nil {
		t.Fatalf("remove cover member: %v", err)
	}

	got, err := st.GetCollection(ctx, owner, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cover != "" {
		t.Fatalf("raw cover after removing cover member = %q, want '' (cleared)", got.Cover)
	}
	// The effective cover falls back to the remaining member, not a dangling ref.
	cols, _ := st.ListCollections(ctx, owner)
	if cols[0].Cover != "a2" {
		t.Fatalf("effective cover after clear = %q, want a2 (fallback)", cols[0].Cover)
	}
}

func TestSQLite_DeleteCollectionCascadesItems(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	c := mkCollection(t, owner, "c1", "C", "")
	if err := st.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	addItem(t, ctx, st, owner, c.ID, a1)

	if err := st.DeleteCollection(ctx, owner, c.ID); err != nil {
		t.Fatal(err)
	}
	// Re-create the collection under the same id: had DeleteCollection left
	// orphaned membership rows, they would resurface here. An empty list proves
	// the cascade removed them.
	if err := st.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	items, err := st.ListCollectionItems(ctx, owner, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("after delete+recreate items = %d, want 0 (no orphans)", len(items))
	}
}
