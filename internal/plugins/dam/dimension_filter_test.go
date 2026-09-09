package dam_test

import (
	"context"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// seedDim upserts a live asset with explicit size/width/height (seedKind leaves
// them at 1/0/0), so the size/dimension/shape filters have real values to narrow
// on. Names embed "photo" so ?q=photo returns the full set on the search path.
func seedDim(t *testing.T, ctx context.Context, st interface {
	UpsertAsset(context.Context, domain.Asset) error
}, owner domain.OwnerID, id, name string, kind domain.AssetKind, size int64, width, height int) domain.AssetID {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: kind, Provider: "local",
		StoragePath: name, Name: name, Ext: "x", Size: size, Hash: "h-" + id,
		Width: width, Height: height, CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	return aid
}

// TestDimensionFilterAPI exercises the issue #101 size/dimension/shape params on
// BOTH /assets and /search?q= (the issue requires identical facet behavior on
// the two paths), plus composition with the existing kind/rating facets and the
// normalizeRange edge cases (negative -> unbounded, inverted -> ignore max, and
// the shape whitelist dropping unknown tokens).
func TestDimensionFilterAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()

	seedDim(t, ctx, st, owner, "a1", "one photo", domain.KindImage, 100, 400, 200)          // landscape, small
	a2 := seedDim(t, ctx, st, owner, "a2", "two photo", domain.KindImage, 900000, 200, 400) // portrait, big
	seedDim(t, ctx, st, owner, "a3", "three photo", domain.KindImage, 5000, 300, 300)       // square, mid
	seedDim(t, ctx, st, owner, "a4", "four photo", domain.KindVideo, 8000, 800, 400)        // landscape video

	// Rate the portrait so an old-facet (rating) x new-facet (shape) case runs.
	if err := st.BatchUpdateRating(ctx, owner, []domain.AssetID{a2}, 5); err != nil {
		t.Fatal(err)
	}

	paths := []struct {
		name string
		base string
		pre  string // prefix that selects the full set on this path
	}{
		{"assets", "/api/dam/assets", ""},
		{"search", "/api/dam/search", "q=photo&"},
	}
	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"minSize", "minSize=1000", 3},
		{"maxSize", "maxSize=1000", 1},
		{"size band", "minSize=1000&maxSize=100000", 2},
		{"minWidth", "minWidth=500", 1},
		{"maxHeight", "maxHeight=250", 1},
		{"minWidth+maxHeight", "minWidth=250&maxHeight=350", 2},
		{"shape landscape", "shape=landscape", 2},
		{"shape portrait", "shape=portrait", 1},
		{"shape multi", "shape=landscape,portrait", 3},
		{"kind+shape combo", "kind=image&shape=landscape", 1},
		{"rating+shape combo", "rating=5&shape=portrait", 1},
		{"neg minSize unbounded", "minSize=-5", 4},
		{"maxWidth<minWidth ignores max", "maxWidth=100&minWidth=500", 1},
		{"unknown shape dropped", "shape=bogus", 4},
		{"shape valid+unknown keeps valid", "shape=landscape,bogus", 2},
	}
	for _, p := range paths {
		for _, tc := range cases {
			t.Run(p.name+"/"+tc.name, func(t *testing.T) {
				url := srv.URL + p.base + "?" + p.pre + tc.query
				if got := getItems(t, url); len(got) != tc.want {
					t.Errorf("GET %s = %d assets, want %d", url, len(got), tc.want)
				}
			})
		}
	}

	// Multi-value shape results carry only the requested buckets (never square).
	for _, a := range getItems(t, srv.URL+"/api/dam/assets?shape=landscape,portrait") {
		if a.Name == "three photo" {
			t.Errorf("shape=landscape,portrait returned the square asset %q", a.Name)
		}
	}
}
