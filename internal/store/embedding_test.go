package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

func TestSQLite_IndexAndGetEmbedding(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	aid := seedAsset(t, ctx, st, owner, "a1", "a.png")

	vec := []float32{0.1, 0.2, 0.3, 0.4}
	if err := st.IndexEmbedding(ctx, aid, vec, "clip-test"); err != nil {
		t.Fatalf("index embedding: %v", err)
	}

	got, err := st.GetEmbedding(ctx, aid)
	if err != nil {
		t.Fatalf("get embedding: %v", err)
	}
	if len(got) != len(vec) {
		t.Fatalf("dim = %d, want %d", len(got), len(vec))
	}
	for i := range vec {
		if got[i] != vec[i] {
			t.Errorf("[%d] = %v, want %v", i, got[i], vec[i])
		}
	}
}

func TestSQLite_GetEmbedding_NotFound(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	aid := seedAsset(t, ctx, st, owner, "a1", "a.png")
	_, err := st.GetEmbedding(ctx, aid)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_IndexEmbedding_Upsert(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	aid := seedAsset(t, ctx, st, owner, "a1", "a.png")

	if err := st.IndexEmbedding(ctx, aid, []float32{1, 0}, "m1"); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexEmbedding(ctx, aid, []float32{0, 1, 0, 0}, "m2"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetEmbedding(ctx, aid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("after upsert dim = %d, want 4 (replaced)", len(got))
	}
}

func TestSQLite_SearchByEmbedding_RanksAndLimits(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	// Three orthogonal unit vectors so dot products are exactly known.
	a := seedAsset(t, ctx, st, owner, "a", "a.png")
	b := seedAsset(t, ctx, st, owner, "b", "b.png")
	c := seedAsset(t, ctx, st, owner, "c", "c.png")
	mustIndexEmbed(t, ctx, st, a, []float32{1, 0, 0})
	mustIndexEmbed(t, ctx, st, b, []float32{0, 1, 0})
	mustIndexEmbed(t, ctx, st, c, []float32{0, 0, 1})

	// Query aligned with a, slightly toward b: expect a first, then b, then c.
	q := []float32{0.9, 0.4, 0.0}
	matches, err := st.SearchByEmbedding(ctx, owner, q, domain.AssetID{}, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("got %d matches, want 3", len(matches))
	}
	if matches[0].Asset.StoragePath != "a.png" || matches[1].Asset.StoragePath != "b.png" {
		t.Fatalf("order = %s,%s,%s want a,b,c",
			matches[0].Asset.StoragePath, matches[1].Asset.StoragePath, matches[2].Asset.StoragePath)
	}
	// Descending similarity.
	if matches[0].Similarity < matches[1].Similarity || matches[1].Similarity < matches[2].Similarity {
		t.Fatalf("not descending: %v", matches)
	}

	// limit truncates.
	lim, err := st.SearchByEmbedding(ctx, owner, q, domain.AssetID{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(lim) != 1 || lim[0].Asset.StoragePath != "a.png" {
		t.Fatalf("limit=1 = %+v, want only a.png", lim)
	}
}

func TestSearchByEmbedding_ExcludesSelf(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a := seedAsset(t, ctx, st, owner, "a", "a.png")
	b := seedAsset(t, ctx, st, owner, "b", "b.png")
	mustIndexEmbed(t, ctx, st, a, []float32{1, 0, 0})
	mustIndexEmbed(t, ctx, st, b, []float32{0, 1, 0})

	// Search by a's own vector, excluding a: b must be the only result.
	matches, err := st.SearchByEmbedding(ctx, owner, []float32{1, 0, 0}, a, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Asset.StoragePath != "b.png" {
		t.Fatalf("excluding self = %+v, want only b.png", matches)
	}
}

func TestSearchByEmbedding_SkipsDimensionMismatch(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a := seedAsset(t, ctx, st, owner, "a", "a.png")
	b := seedAsset(t, ctx, st, owner, "b", "b.png")
	mustIndexEmbed(t, ctx, st, a, []float32{1, 0, 0}) // dim 3
	mustIndexEmbed(t, ctx, st, b, []float32{1, 0})    // dim 2 (mismatch)

	matches, err := st.SearchByEmbedding(ctx, owner, []float32{1, 0, 0}, domain.AssetID{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Asset.StoragePath != "a.png" {
		t.Fatalf("mismatch skip = %+v, want only a.png", matches)
	}
}

func TestSearchByEmbedding_ExcludesTrashed(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a := seedAsset(t, ctx, st, owner, "a", "a.png")
	b := seedAsset(t, ctx, st, owner, "b", "b.png")
	mustIndexEmbed(t, ctx, st, a, []float32{1, 0, 0})
	mustIndexEmbed(t, ctx, st, b, []float32{1, 0, 0})

	// Trash b: its embedding must not leak into results.
	if err := st.BatchTrashAssets(ctx, owner, []domain.AssetID{b}); err != nil {
		t.Fatal(err)
	}
	matches, err := st.SearchByEmbedding(ctx, owner, []float32{1, 0, 0}, domain.AssetID{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Asset.StoragePath != "a.png" {
		t.Fatalf("trashed leak = %+v, want only a.png", matches)
	}
}

func TestSearchByEmbedding_OwnerIsolation(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a := seedAsset(t, ctx, st, owner, "a", "a.png")
	mustIndexEmbed(t, ctx, st, a, []float32{1, 0, 0})

	other, err := domain.NewOwnerID("other")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, other); err != nil {
		t.Fatal(err)
	}
	matches, err := st.SearchByEmbedding(ctx, other, []float32{1, 0, 0}, domain.AssetID{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("cross-owner leak = %+v, want none", matches)
	}
}

func TestSearchByEmbedding_EmptyStore(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	matches, err := st.SearchByEmbedding(ctx, owner, []float32{1, 0, 0}, domain.AssetID{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("empty store = %+v, want none", matches)
	}
}

func mustIndexEmbed(t *testing.T, ctx context.Context, st *store.SQLite, aid domain.AssetID, vec []float32) {
	t.Helper()
	if err := st.IndexEmbedding(ctx, aid, vec, "clip-test"); err != nil {
		t.Fatalf("index embed %s: %v", aid, err)
	}
}
