package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// seedDimAsset upserts a live asset with explicit size/width/height, which the
// shared seedAsset/seedKindAsset helpers leave at their defaults — the size,
// dimension, and shape filters need real values to narrow on (issue #101).
func seedDimAsset(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id, name string, kind domain.AssetKind, size int64, width, height int) domain.AssetID {
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

// listDimNames runs a size/dimension/shape filter and returns matching names,
// so the table cases below can assert against a plain name set.
func listDimNames(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, f domain.AssetFilter) []string {
	t.Helper()
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

// TestListAssetsFilteredBySize covers the a.size range condition (anchor row,
// not version-resolved) added to appendFacetConds for issue #101.
func TestListAssetsFilteredBySize(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedDimAsset(t, ctx, st, owner, "s1", "small.png", domain.KindImage, 100, 10, 10)
	seedDimAsset(t, ctx, st, owner, "s2", "mid.png", domain.KindImage, 5000, 10, 10)
	seedDimAsset(t, ctx, st, owner, "s3", "big.png", domain.KindImage, 1_000_000, 10, 10)

	cases := []struct {
		name string
		f    domain.AssetFilter
		want []string
	}{
		{"min only", domain.AssetFilter{MinSize: 5000}, []string{"mid.png", "big.png"}},
		{"max only", domain.AssetFilter{MaxSize: 5000}, []string{"small.png", "mid.png"}},
		{"band", domain.AssetFilter{MinSize: 200, MaxSize: 100_000}, []string{"mid.png"}},
		{"no match", domain.AssetFilter{MinSize: 2_000_000}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNameSet(t, listDimNames(t, ctx, st, owner, tc.f), tc.want)
		})
	}
}

// TestListAssetsFilteredByDimensions covers the COALESCE(cv.width/height,
// a.width/height) range conditions, each dimension independently and combined.
func TestListAssetsFilteredByDimensions(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedDimAsset(t, ctx, st, owner, "d1", "wide.png", domain.KindImage, 1, 800, 100)
	seedDimAsset(t, ctx, st, owner, "d2", "tall.png", domain.KindImage, 1, 100, 800)
	seedDimAsset(t, ctx, st, owner, "d3", "mid.png", domain.KindImage, 1, 400, 400)

	cases := []struct {
		name string
		f    domain.AssetFilter
		want []string
	}{
		{"minWidth", domain.AssetFilter{MinWidth: 500}, []string{"wide.png"}},
		{"maxWidth", domain.AssetFilter{MaxWidth: 200}, []string{"tall.png"}},
		{"width band", domain.AssetFilter{MinWidth: 300, MaxWidth: 500}, []string{"mid.png"}},
		{"minHeight", domain.AssetFilter{MinHeight: 500}, []string{"tall.png"}},
		{"maxHeight", domain.AssetFilter{MaxHeight: 200}, []string{"wide.png"}},
		{"height band", domain.AssetFilter{MinHeight: 300, MaxHeight: 500}, []string{"mid.png"}},
		{"width+height", domain.AssetFilter{MinWidth: 300, MaxWidth: 500, MinHeight: 300, MaxHeight: 500}, []string{"mid.png"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNameSet(t, listDimNames(t, ctx, st, owner, tc.f), tc.want)
		})
	}
}

// TestListAssetsFilteredByShape covers the aspect-ratio buckets: each shape in
// isolation, a multi-select OR, and the shared zero-dimension guard that keeps
// audio/document assets (width=0 or height=0) out of every bucket.
func TestListAssetsFilteredByShape(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	// 400/200=2.0>=1.1 landscape; 200/400=0.5<=0.9 portrait; 300/300=1.0 square.
	seedDimAsset(t, ctx, st, owner, "l1", "land.png", domain.KindImage, 1, 400, 200)
	seedDimAsset(t, ctx, st, owner, "p1", "port.png", domain.KindImage, 1, 200, 400)
	seedDimAsset(t, ctx, st, owner, "q1", "square.png", domain.KindImage, 1, 300, 300)
	// Zero-dimension audio must fall in NO bucket even when all shapes asked.
	seedDimAsset(t, ctx, st, owner, "z1", "beat.mp3", domain.KindAudio, 1, 0, 0)

	cases := []struct {
		name string
		f    domain.AssetFilter
		want []string
	}{
		{"landscape", domain.AssetFilter{Shapes: []domain.AssetShape{domain.ShapeLandscape}}, []string{"land.png"}},
		{"portrait", domain.AssetFilter{Shapes: []domain.AssetShape{domain.ShapePortrait}}, []string{"port.png"}},
		{"square", domain.AssetFilter{Shapes: []domain.AssetShape{domain.ShapeSquare}}, []string{"square.png"}},
		{"landscape+portrait OR", domain.AssetFilter{Shapes: []domain.AssetShape{domain.ShapeLandscape, domain.ShapePortrait}}, []string{"land.png", "port.png"}},
		{"all three exclude zero-dim", domain.AssetFilter{Shapes: domain.AllShapes}, []string{"land.png", "port.png", "square.png"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNameSet(t, listDimNames(t, ctx, st, owner, tc.f), tc.want)
		})
	}
}

// TestListAssetsFilteredByDimensionsResolvesCurrentVersion locks the issue's
// acceptance criterion: width/height filter the CURRENT version, not the anchor.
// addTwoVersions makes v2 (100x80) current over v1 (320x240).
func TestListAssetsFilteredByDimensionsResolvesCurrentVersion(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	addTwoVersions(t, ctx, st, owner)

	// A band around the current version (v2=100) returns the asset.
	if got := listDimNames(t, ctx, st, owner, domain.AssetFilter{MinWidth: 90, MaxWidth: 110}); len(got) != 1 {
		t.Fatalf("MinWidth 90-110 (current v2=100) = %v, want the asset", got)
	}
	// A band around only the original version (v1=320) must NOT return it —
	// proving resolution is the current version, not the anchor/original.
	if got := listDimNames(t, ctx, st, owner, domain.AssetFilter{MinWidth: 300, MaxWidth: 340}); len(got) != 0 {
		t.Fatalf("MinWidth 300-340 (original v1=320) = %v, want none (current is v2=100)", got)
	}
}

// TestKindCountsWithDimensionFilter proves the currentVersionJoin added to
// KindCounts lets size/dimension/shape filters narrow the per-kind counts
// without the 1:1 cv join duplicating rows or breaking the GROUP BY.
func TestKindCountsWithDimensionFilter(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedDimAsset(t, ctx, st, owner, "k1", "wide1.png", domain.KindImage, 1, 800, 100)
	seedDimAsset(t, ctx, st, owner, "k2", "wide2.png", domain.KindImage, 1, 800, 100)
	seedDimAsset(t, ctx, st, owner, "k3", "tall.mp4", domain.KindVideo, 1, 100, 800)

	// MinWidth 500 keeps the two wide images; the tall video (width 100) drops.
	counts, err := st.KindCounts(ctx, owner, domain.AssetFilter{MinWidth: 500})
	if err != nil {
		t.Fatalf("kind counts dim: %v", err)
	}
	if counts[domain.KindImage] != 2 || counts[domain.KindVideo] != 0 {
		t.Errorf("dim counts = %+v, want image:2 video:0 (no row inflation)", counts)
	}
	// A landscape shape + MaxHeight filter narrows the counts the same way,
	// exercising the cv alias through the shape branch too.
	shaped, err := st.KindCounts(ctx, owner, domain.AssetFilter{
		MaxHeight: 200, Shapes: []domain.AssetShape{domain.ShapeLandscape},
	})
	if err != nil {
		t.Fatalf("kind counts shape: %v", err)
	}
	if shaped[domain.KindImage] != 2 || shaped[domain.KindVideo] != 0 {
		t.Errorf("shape counts = %+v, want image:2 video:0", shaped)
	}
}
