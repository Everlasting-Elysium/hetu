package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func TestListAssetsFiltered(t *testing.T) {
	ctx, st, owner := mustOpen(t)

	a1 := seedAsset(t, ctx, st, owner, "a1", "one.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "two.png")
	a3 := seedAsset(t, ctx, st, owner, "a3", "three.png")
	_ = seedAsset(t, ctx, st, owner, "a4", "four.png")

	// folder f1 = {a1, a2}; ratings a2=5, a3=3; tag t1 = {a1, a3}.
	if err := st.BatchMoveToFolder(ctx, owner, []domain.AssetID{a1, a2}, "f1"); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchUpdateRating(ctx, owner, []domain.AssetID{a2}, 5); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchUpdateRating(ctx, owner, []domain.AssetID{a3}, 3); err != nil {
		t.Fatal(err)
	}
	tid, err := domain.NewTagID("t1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTag(ctx, domain.Tag{ID: tid, Owner: owner, Name: "landscape"}); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{a1, a3}, []domain.TagID{tid}); err != nil {
		t.Fatal(err)
	}

	list := func(f domain.AssetFilter) []string {
		assets, err := st.ListAssetsFiltered(ctx, owner, f, 50, 0)
		if err != nil {
			t.Fatalf("filtered %+v: %v", f, err)
		}
		names := make([]string, len(assets))
		for i, a := range assets {
			names[i] = a.Name
		}
		return names
	}

	cases := []struct {
		name string
		f    domain.AssetFilter
		want []string
	}{
		{"no filter", domain.AssetFilter{}, []string{"one.png", "two.png", "three.png", "four.png"}},
		{"folder", domain.AssetFilter{FolderID: "f1"}, []string{"one.png", "two.png"}},
		{"rating>=3", domain.AssetFilter{MinRating: 3}, []string{"two.png", "three.png"}},
		{"rating>=5", domain.AssetFilter{MinRating: 5}, []string{"two.png"}},
		{"tag", domain.AssetFilter{TagID: "t1"}, []string{"one.png", "three.png"}},
		{"folder+rating", domain.AssetFilter{FolderID: "f1", MinRating: 5}, []string{"two.png"}},
		{"folder+tag", domain.AssetFilter{FolderID: "f1", TagID: "t1"}, []string{"one.png"}},
		{"no match", domain.AssetFilter{FolderID: "nope"}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNameSet(t, list(tc.f), tc.want)
		})
	}
}

// assertNameSet compares two name lists as sets (order is unspecified because
// seeded assets share an indexed_at second).
func assertNameSet(t *testing.T, got, want []string) {
	t.Helper()
	m := make(map[string]int, len(got))
	for _, s := range got {
		m[s]++
	}
	for _, s := range want {
		m[s]--
	}
	for k, v := range m {
		if v != 0 {
			t.Errorf("got %v, want set %v (mismatch on %q)", got, want, k)
			return
		}
	}
}

// seedKindAsset upserts a live asset of the given kind (seedAsset is image-only).
func seedKindAsset(t *testing.T, ctx context.Context, st interface {
	UpsertAsset(context.Context, domain.Asset) error
}, owner domain.OwnerID, id, name string, kind domain.AssetKind) domain.AssetID {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: kind, Provider: "local",
		StoragePath: name, Name: name, Ext: "x", Size: 1, Hash: "h-" + id,
		CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	return aid
}

// TestListAssetsFilteredByKind covers the a.kind IN (...) condition, single and
// multi-value, through the shared appendFacetConds path (issue #75).
func TestListAssetsFilteredByKind(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedKindAsset(t, ctx, st, owner, "a1", "one.png", domain.KindImage)
	seedKindAsset(t, ctx, st, owner, "a2", "two.mp4", domain.KindVideo)
	seedKindAsset(t, ctx, st, owner, "a3", "three.png", domain.KindImage)

	count := func(kinds ...domain.AssetKind) int {
		got, err := st.ListAssetsFiltered(ctx, owner, domain.AssetFilter{Kinds: kinds}, 50, 0)
		if err != nil {
			t.Fatalf("list kinds %v: %v", kinds, err)
		}
		return len(got)
	}
	if n := count(domain.KindImage); n != 2 {
		t.Errorf("image = %d, want 2", n)
	}
	if n := count(domain.KindVideo); n != 1 {
		t.Errorf("video = %d, want 1", n)
	}
	if n := count(domain.KindImage, domain.KindVideo); n != 3 {
		t.Errorf("image+video = %d, want 3", n)
	}
	if n := count(domain.KindAudio); n != 0 {
		t.Errorf("audio = %d, want 0", n)
	}
}

// TestKindCounts covers the GROUP BY count query and its rule that the kind
// facet is ignored while counting (so every format stays selectable), but other
// facets (rating) still narrow the counts.
func TestKindCounts(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedKindAsset(t, ctx, st, owner, "a1", "one.png", domain.KindImage)
	seedKindAsset(t, ctx, st, owner, "a2", "two.png", domain.KindImage)
	vid := seedKindAsset(t, ctx, st, owner, "a3", "three.mp4", domain.KindVideo)
	if err := st.BatchUpdateRating(ctx, owner, []domain.AssetID{vid}, 5); err != nil {
		t.Fatal(err)
	}

	counts, err := st.KindCounts(ctx, owner, domain.AssetFilter{})
	if err != nil {
		t.Fatalf("kind counts: %v", err)
	}
	if counts[domain.KindImage] != 2 || counts[domain.KindVideo] != 1 {
		t.Errorf("counts = %+v, want image:2 video:1", counts)
	}
	// Kinds is ignored (video still counted) while MinRating narrows images out.
	rated, err := st.KindCounts(ctx, owner, domain.AssetFilter{
		Kinds: []domain.AssetKind{domain.KindImage}, MinRating: 5,
	})
	if err != nil {
		t.Fatalf("kind counts rated: %v", err)
	}
	if rated[domain.KindImage] != 0 || rated[domain.KindVideo] != 1 {
		t.Errorf("rated counts = %+v, want image:0 video:1", rated)
	}
}
