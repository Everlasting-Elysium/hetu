package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

func mkCollection(t *testing.T, owner domain.OwnerID, id, name, parentID string) domain.Collection {
	t.Helper()
	cid, err := domain.NewCollectionID(id)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Collection{ID: cid, Owner: owner, Name: name, ParentID: parentID}
}

// addItem appends an asset to a collection, failing the test on error.
func addItem(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, cid domain.CollectionID, aid domain.AssetID) {
	t.Helper()
	if err := st.AddCollectionItem(ctx, owner, cid, aid); err != nil {
		t.Fatalf("add collection item %s: %v", aid, err)
	}
}

func TestSQLite_CollectionCRUD(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a := mkCollection(t, owner, "c1", "Alpha", "")
	b := mkCollection(t, owner, "c2", "Beta", "")
	if err := st.CreateCollection(ctx, a); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if err := st.CreateCollection(ctx, b); err != nil {
		t.Fatalf("create beta: %v", err)
	}

	cols, err := st.ListCollections(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	// Ordered by name: Alpha, Beta.
	if len(cols) != 2 || cols[0].Name != "Alpha" || cols[1].Name != "Beta" {
		t.Fatalf("list = %+v, want [Alpha Beta]", cols)
	}

	got, err := st.GetCollection(ctx, owner, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Alpha" {
		t.Fatalf("get name = %q, want Alpha", got.Name)
	}

	if err := st.UpdateCollection(ctx, owner, a.ID, "Renamed", "", ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = st.GetCollection(ctx, owner, a.ID)
	if got.Name != "Renamed" {
		t.Fatalf("after update name = %q, want Renamed", got.Name)
	}

	if err := st.DeleteCollection(ctx, owner, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetCollection(ctx, owner, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_CollectionNesting(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	parent := mkCollection(t, owner, "p", "Parent", "")
	if err := st.CreateCollection(ctx, parent); err != nil {
		t.Fatal(err)
	}
	child := mkCollection(t, owner, "c", "Child", parent.ID.String())
	if err := st.CreateCollection(ctx, child); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCollection(ctx, owner, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ParentID != "p" {
		t.Fatalf("child parent = %q, want p", got.ParentID)
	}
}

func TestSQLite_CollectionCoverFallback(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	c := mkCollection(t, owner, "c1", "C", "")
	if err := st.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	// Empty collection: no cover.
	cols, _ := st.ListCollections(ctx, owner)
	if cols[0].Cover != "" {
		t.Fatalf("empty cover = %q, want ''", cols[0].Cover)
	}
	// Members, no override: lowest-ord member (a1).
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")
	addItem(t, ctx, st, owner, c.ID, a1)
	addItem(t, ctx, st, owner, c.ID, a2)
	cols, _ = st.ListCollections(ctx, owner)
	if cols[0].Cover != "a1" {
		t.Fatalf("fallback cover = %q, want a1", cols[0].Cover)
	}
	// Manual override to a member (a2).
	if err := st.UpdateCollection(ctx, owner, c.ID, "C", "", "a2"); err != nil {
		t.Fatalf("set cover: %v", err)
	}
	cols, _ = st.ListCollections(ctx, owner)
	if cols[0].Cover != "a2" {
		t.Fatalf("override cover = %q, want a2", cols[0].Cover)
	}
	// GetCollection returns the RAW override, not the resolved value.
	got, _ := st.GetCollection(ctx, owner, c.ID)
	if got.Cover != "a2" {
		t.Fatalf("raw cover = %q, want a2", got.Cover)
	}
	// Clearing the override falls back to the lowest-ord member again.
	if err := st.UpdateCollection(ctx, owner, c.ID, "C", "", ""); err != nil {
		t.Fatalf("clear cover: %v", err)
	}
	cols, _ = st.ListCollections(ctx, owner)
	if cols[0].Cover != "a1" {
		t.Fatalf("cleared cover = %q, want a1 (fallback)", cols[0].Cover)
	}
}

func TestSQLite_CollectionParentSelfRejected(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	c := mkCollection(t, owner, "c1", "C", "")
	if err := st.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	err := st.UpdateCollection(ctx, owner, c.ID, "C", "c1", "")
	if !errors.Is(err, domain.ErrCollectionCycle) {
		t.Fatalf("self-parent err = %v, want ErrCollectionCycle", err)
	}
}

func TestSQLite_CollectionParentCycleRejected(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a := mkCollection(t, owner, "a", "A", "")
	if err := st.CreateCollection(ctx, a); err != nil {
		t.Fatal(err)
	}
	b := mkCollection(t, owner, "b", "B", "a") // b is a's child
	if err := st.CreateCollection(ctx, b); err != nil {
		t.Fatal(err)
	}
	// Reparenting a under its own descendant b must be rejected.
	err := st.UpdateCollection(ctx, owner, a.ID, "A", "b", "")
	if !errors.Is(err, domain.ErrCollectionCycle) {
		t.Fatalf("cycle err = %v, want ErrCollectionCycle", err)
	}
	// a must remain a root (rejected update left it untouched).
	got, _ := st.GetCollection(ctx, owner, a.ID)
	if got.ParentID != "" {
		t.Fatalf("a.ParentID = %q after rejected cycle, want unchanged ''", got.ParentID)
	}
}

func TestSQLite_CollectionParentMustExist(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	c := mkCollection(t, owner, "c1", "C", "does-not-exist")
	err := st.CreateCollection(ctx, c)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing parent err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_CollectionCoverNonMemberRejected(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	c := mkCollection(t, owner, "c1", "C", "")
	if err := st.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	seedAsset(t, ctx, st, owner, "a2", "2.png") // seeded but never added
	addItem(t, ctx, st, owner, c.ID, a1)
	err := st.UpdateCollection(ctx, owner, c.ID, "C", "", "a2")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cover non-member err = %v, want ErrNotFound", err)
	}
}
