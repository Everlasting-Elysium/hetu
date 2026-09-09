package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// seedDurAsset upserts a live asset of kind and attaches its duration as an
// extracted-layer annotation — a json-marshaled float64 seconds under key,
// exactly what the audio/video handlers write through IndexMetadata — so the
// duration facet has a real value to range over (issue #53). key is
// domain.KeyAudioDuration or domain.KeyVideoDuration; seconds <= 0 attaches no
// annotation (mirroring an asset the probe found no duration for, e.g. an image).
func seedDurAsset(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id, name string, kind domain.AssetKind, key string, seconds float64) domain.AssetID {
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
	if seconds > 0 {
		val, err := json.Marshal(seconds)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpsertAnnotation(ctx, owner, domain.Annotation{
			AssetID: aid, Layer: domain.LayerExtracted, Key: key, Value: string(val),
		}); err != nil {
			t.Fatalf("seed duration %s: %v", id, err)
		}
	}
	return aid
}

// seedTimeAsset upserts a live image asset with explicit created/indexed unix
// seconds, since the shared seed helpers stamp everything with time.Now and the
// time-range filters need distinct, known timestamps to narrow on (issue #53).
func seedTimeAsset(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id, name string, createdAt, indexedAt int64) domain.AssetID {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: name, Name: name, Ext: "png", Size: 1, Hash: "h-" + id,
		CreatedAt: time.Unix(createdAt, 0).UTC(), IndexedAt: time.Unix(indexedAt, 0).UTC(),
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	return aid
}

// seedDurSet is the shared fixture for the duration cases: two audio + two video
// clips spanning 10..300s, plus a durationless image that must never fall into
// any duration band (adur.value is NULL for it).
func seedDurSet(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID) {
	t.Helper()
	seedDurAsset(t, ctx, st, owner, "d1", "short.mp3", domain.KindAudio, domain.KeyAudioDuration, 10)
	seedDurAsset(t, ctx, st, owner, "d2", "mid.mp3", domain.KindAudio, domain.KeyAudioDuration, 60)
	seedDurAsset(t, ctx, st, owner, "d3", "clip.mp4", domain.KindVideo, domain.KeyVideoDuration, 120)
	seedDurAsset(t, ctx, st, owner, "d4", "long.mp4", domain.KindVideo, domain.KeyVideoDuration, 300)
	seedDurAsset(t, ctx, st, owner, "d5", "photo.png", domain.KindImage, "", 0) // no duration
}

// TestListAssetsFilteredByDuration covers the CAST(adur.value AS REAL) range
// condition and its durationJoin: each bound alone, a band, no-match, that a
// durationless image never matches, and that both audio and video are ranged
// over by the one annotation query (issue #53).
func TestListAssetsFilteredByDuration(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedDurSet(t, ctx, st, owner)

	cases := []struct {
		name string
		f    domain.AssetFilter
		want []string
	}{
		{"min only", domain.AssetFilter{MinDuration: 60}, []string{"mid.mp3", "clip.mp4", "long.mp4"}},
		{"max only", domain.AssetFilter{MaxDuration: 60}, []string{"short.mp3", "mid.mp3"}},
		{"band", domain.AssetFilter{MinDuration: 30, MaxDuration: 200}, []string{"mid.mp3", "clip.mp4"}},
		{"no match", domain.AssetFilter{MinDuration: 1000}, []string{}},
		// A band straddling the audio/video divide returns one of each, proving
		// both KeyAudioDuration and KeyVideoDuration are ranged over together.
		{"audio+video both hit", domain.AssetFilter{MinDuration: 60, MaxDuration: 120}, []string{"mid.mp3", "clip.mp4"}},
		// Any bound at all excludes the NULL-duration image without a zero-guard.
		{"max excludes durationless image", domain.AssetFilter{MaxDuration: 100000}, []string{"short.mp3", "mid.mp3", "clip.mp4", "long.mp4"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNameSet(t, listDimNames(t, ctx, st, owner, tc.f), tc.want)
		})
	}

	// No duration filter at all lists every asset, the image included — 0 means
	// "no bound", so the durationJoin is present but adds no condition.
	assertNameSet(t, listDimNames(t, ctx, st, owner, domain.AssetFilter{}),
		[]string{"short.mp3", "mid.mp3", "clip.mp4", "long.mp4", "photo.png"})
}

// TestListAssetsFilteredByTime covers the a.created_at / a.indexed_at range
// conditions (unix seconds, no join): each bound and band on both columns,
// independently and combined (issue #53).
func TestListAssetsFilteredByTime(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedTimeAsset(t, ctx, st, owner, "t1", "jan.png", 100, 1000)
	seedTimeAsset(t, ctx, st, owner, "t2", "feb.png", 200, 2000)
	seedTimeAsset(t, ctx, st, owner, "t3", "mar.png", 300, 3000)

	cases := []struct {
		name string
		f    domain.AssetFilter
		want []string
	}{
		{"createdAfter", domain.AssetFilter{CreatedAfter: 200}, []string{"feb.png", "mar.png"}},
		{"createdBefore", domain.AssetFilter{CreatedBefore: 200}, []string{"jan.png", "feb.png"}},
		{"created band", domain.AssetFilter{CreatedAfter: 150, CreatedBefore: 250}, []string{"feb.png"}},
		{"indexedAfter", domain.AssetFilter{IndexedAfter: 2000}, []string{"feb.png", "mar.png"}},
		{"indexedBefore", domain.AssetFilter{IndexedBefore: 2000}, []string{"jan.png", "feb.png"}},
		{"indexed band", domain.AssetFilter{IndexedAfter: 1500, IndexedBefore: 2500}, []string{"feb.png"}},
		// created and indexed compose (AND): createdAfter keeps feb+mar, then
		// indexedBefore drops mar, leaving feb — the two columns narrow together.
		{"created+indexed combo", domain.AssetFilter{CreatedAfter: 200, IndexedBefore: 2000}, []string{"feb.png"}},
		{"no match", domain.AssetFilter{CreatedAfter: 9999}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNameSet(t, listDimNames(t, ctx, st, owner, tc.f), tc.want)
		})
	}
}

// TestKindCountsWithDurationFilter proves the durationJoin added to KindCounts
// lets the duration condition narrow the per-kind counts without the adur join
// duplicating rows or breaking the GROUP BY — the same lesson as #101's cv join.
func TestKindCountsWithDurationFilter(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedDurSet(t, ctx, st, owner)

	// MinDuration 100 keeps the two videos (120/300); both audios (10/60) and
	// the durationless image drop.
	counts, err := st.KindCounts(ctx, owner, domain.AssetFilter{MinDuration: 100})
	if err != nil {
		t.Fatalf("kind counts duration: %v", err)
	}
	if counts[domain.KindVideo] != 2 || counts[domain.KindAudio] != 0 || counts[domain.KindImage] != 0 {
		t.Errorf("duration counts = %+v, want video:2 audio:0 image:0 (no row inflation)", counts)
	}

	// A band that spans both audio and video narrows to one of each, exercising
	// the adur alias across both duration keys through the counts path too.
	band, err := st.KindCounts(ctx, owner, domain.AssetFilter{MinDuration: 60, MaxDuration: 120})
	if err != nil {
		t.Fatalf("kind counts duration band: %v", err)
	}
	if band[domain.KindAudio] != 1 || band[domain.KindVideo] != 1 {
		t.Errorf("duration band counts = %+v, want audio:1 video:1", band)
	}
}

// TestKindCountsWithTimeFilter proves the created/indexed conditions narrow the
// per-kind counts through the shared appendFacetConds (no join needed).
func TestKindCountsWithTimeFilter(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	seedTimeAsset(t, ctx, st, owner, "t1", "jan.png", 100, 1000)
	seedTimeAsset(t, ctx, st, owner, "t2", "feb.png", 200, 2000)
	seedTimeAsset(t, ctx, st, owner, "t3", "mar.png", 300, 3000)

	// CreatedAfter 200 keeps feb + mar (both images); jan drops. The time
	// condition narrows the per-kind counts through the shared appendFacetConds.
	counts, err := st.KindCounts(ctx, owner, domain.AssetFilter{CreatedAfter: 200})
	if err != nil {
		t.Fatalf("kind counts time: %v", err)
	}
	if counts[domain.KindImage] != 2 {
		t.Errorf("time counts = %+v, want image:2", counts)
	}
}
