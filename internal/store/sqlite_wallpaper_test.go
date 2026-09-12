package store_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// mustCollectionID parses a collection id, failing the test on error.
func mustCollectionID(t *testing.T, s string) domain.CollectionID {
	t.Helper()
	id, err := domain.NewCollectionID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestCountAssetsFiltered covers the wallpaper daily-pick population count
// (issue #114): unlike KindCounts it HONORS f.Kinds, and it applies the same
// status/facet conditions as ListAssetsFiltered.
func TestCountAssetsFiltered(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedKindAsset(t, ctx, st, owner, "a1", "one.png", domain.KindImage)
	seedKindAsset(t, ctx, st, owner, "a2", "two.png", domain.KindImage)
	seedKindAsset(t, ctx, st, owner, "a3", "three.mp4", domain.KindVideo)

	count := func(f domain.AssetFilter) int {
		n, err := st.CountAssetsFiltered(ctx, owner, f)
		if err != nil {
			t.Fatalf("count %+v: %v", f, err)
		}
		return n
	}
	if n := count(domain.AssetFilter{}); n != 3 {
		t.Errorf("total = %d, want 3", n)
	}
	if n := count(domain.AssetFilter{Kinds: []domain.AssetKind{domain.KindImage}}); n != 2 {
		t.Errorf("image = %d, want 2", n)
	}
	if n := count(domain.AssetFilter{Kinds: []domain.AssetKind{domain.KindImage, domain.KindVideo}}); n != 3 {
		t.Errorf("image+video = %d, want 3", n)
	}
	if n := count(domain.AssetFilter{Kinds: []domain.AssetKind{domain.KindAudio}}); n != 0 {
		t.Errorf("audio = %d, want 0", n)
	}
}

// TestListCollectionAssets covers the full-asset, ord-ordered collection read
// (issue #114): it returns width/height/size the lightweight CollectionItem
// lacks, in manual order, and 404s for a missing collection.
func TestListCollectionAssets(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedDimAsset(t, ctx, st, owner, "a1", "one.png", domain.KindImage, 111, 800, 600)
	a2 := seedDimAsset(t, ctx, st, owner, "a2", "two.png", domain.KindImage, 222, 1920, 1080)
	_ = seedDimAsset(t, ctx, st, owner, "a3", "three.png", domain.KindImage, 333, 100, 100)

	cid := mustCollectionID(t, "c1")
	if err := st.CreateCollection(ctx, domain.Collection{ID: cid, Owner: owner, Name: "wallpapers"}); err != nil {
		t.Fatal(err)
	}
	// Add a2 first, then a1: ord a2=0, a1=1.
	if err := st.AddCollectionItem(ctx, owner, cid, a2); err != nil {
		t.Fatal(err)
	}
	if err := st.AddCollectionItem(ctx, owner, cid, a1); err != nil {
		t.Fatal(err)
	}

	assets, err := st.ListCollectionAssets(ctx, owner, cid, 50, 0)
	if err != nil {
		t.Fatalf("list collection assets: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("got %d assets, want 2", len(assets))
	}
	if assets[0].ID != a2 || assets[1].ID != a1 {
		t.Errorf("order = [%s %s], want ord [a2 a1]", assets[0].ID, assets[1].ID)
	}
	if assets[0].Width != 1920 || assets[0].Height != 1080 || assets[0].Size != 222 {
		t.Errorf("a2 dims = %dx%d size %d, want 1920x1080 size 222", assets[0].Width, assets[0].Height, assets[0].Size)
	}
	if _, err := st.ListCollectionAssets(ctx, owner, mustCollectionID(t, "missing"), 50, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing collection err = %v, want ErrNotFound", err)
	}
}

// TestListCollectionAssetsCrossOwner locks owner isolation: owner A must not be
// able to read owner B's collection — the GetCollection owner scope makes it
// ErrNotFound, so another owner's members never leak through the wallpaper view.
func TestListCollectionAssetsCrossOwner(t *testing.T) {
	ctx, st, ownerA := mustOpen(t)
	ownerB, err := domain.NewOwnerID("other")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, ownerB); err != nil {
		t.Fatal(err)
	}
	bAsset := seedAsset(t, ctx, st, ownerB, "b1", "b.png")
	bColl := mustCollectionID(t, "bc1")
	if err := st.CreateCollection(ctx, domain.Collection{ID: bColl, Owner: ownerB, Name: "b-only"}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddCollectionItem(ctx, ownerB, bColl, bAsset); err != nil {
		t.Fatal(err)
	}

	if _, err := st.ListCollectionAssets(ctx, ownerA, bColl, 50, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("cross-owner read err = %v, want ErrNotFound (no leak)", err)
	}
	// Sanity: the true owner still reads it.
	got, err := st.ListCollectionAssets(ctx, ownerB, bColl, 50, 0)
	if err != nil {
		t.Fatalf("owner B read: %v", err)
	}
	if len(got) != 1 || got[0].ID != bAsset {
		t.Errorf("owner B collection = %v, want [b1]", got)
	}
}

// seedSorted upserts an image with an explicit rating and indexed time so the
// sort assertions below are strict (no tie-breaking ambiguity).
func seedSorted(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id, name string, rating int, indexed time.Time) domain.AssetID {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: name, Name: name, Ext: "png", Size: 1, Hash: "h-" + id,
		Rating: rating, CreatedAt: indexed, IndexedAt: indexed,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	return aid
}

// TestListAssetsFilteredSort covers orderByClause (issue #114): latest orders by
// indexed_at DESC, rating by rating DESC then indexed_at DESC, and random still
// returns the whole matching set (order unchecked).
func TestListAssetsFilteredSort(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	base := time.Now().UTC().Truncate(time.Second)
	a1 := seedSorted(t, ctx, st, owner, "a1", "one.png", 1, base)
	a2 := seedSorted(t, ctx, st, owner, "a2", "two.png", 5, base.Add(time.Second))
	a3 := seedSorted(t, ctx, st, owner, "a3", "three.png", 3, base.Add(2*time.Second))

	ids := func(sort domain.AssetSort) []string {
		assets, err := st.ListAssetsFiltered(ctx, owner, domain.AssetFilter{Sort: sort}, 50, 0)
		if err != nil {
			t.Fatalf("list sort %q: %v", sort, err)
		}
		out := make([]string, len(assets))
		for i, a := range assets {
			out[i] = a.ID.String()
		}
		return out
	}
	if got := ids(domain.SortLatest); !reflect.DeepEqual(got, []string{a3.String(), a2.String(), a1.String()}) {
		t.Errorf("latest = %v, want [a3 a2 a1]", got)
	}
	if got := ids(domain.SortRating); !reflect.DeepEqual(got, []string{a2.String(), a3.String(), a1.String()}) {
		t.Errorf("rating = %v, want [a2 a3 a1]", got)
	}
	if got := ids(domain.SortRandom); len(got) != 3 {
		t.Errorf("random returned %d assets, want 3", len(got))
	}
}
