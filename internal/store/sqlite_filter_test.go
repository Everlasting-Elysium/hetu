package store_test

import (
	"testing"

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
