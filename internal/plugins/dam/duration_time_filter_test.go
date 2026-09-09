package dam_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// seedDur upserts a live asset of kind through the kernel.Store the dam plugin
// sees and, when seconds > 0, attaches its duration as an extracted-layer
// annotation (the json-marshaled float64 the audio/video handlers write via
// IndexMetadata). Names embed "media" so ?q=media returns the full set on the
// search path, letting one fixture drive both /assets and /search (issue #53).
func seedDur(t *testing.T, ctx context.Context, st kernel.Store, owner domain.OwnerID, id, name string, kind domain.AssetKind, key string, seconds float64) {
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
}

// seedTimeAPI upserts a live image asset with explicit created/indexed unix
// seconds and a searchable "media" name, so the time-range params can be
// exercised on both the list and search paths (issue #53).
func seedTimeAPI(t *testing.T, ctx context.Context, st kernel.Store, owner domain.OwnerID, id, name string, createdAt, indexedAt int64) domain.AssetID {
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

// facetPaths runs the same query string on both /assets and /search?q=media so
// each case asserts the two paths narrow identically (the issue's cross-path
// consistency requirement) — the search prefix selects the full "media" set.
var facetPaths = []struct{ name, base, pre string }{
	{"assets", "/api/dam/assets", ""},
	{"search", "/api/dam/search", "q=media&"},
}

// TestDurationFilterAPI exercises ?minDuration=/?maxDuration= on both /assets and
// /search?q=, plus float parsing, kind composition, the durationless-image
// exclusion, that audio and video are both ranged over, and the normalizeRange
// edge cases (negative -> unbounded, inverted -> ignore max) (issue #53).
func TestDurationFilterAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()

	seedDur(t, ctx, st, owner, "a1", "short media", domain.KindAudio, domain.KeyAudioDuration, 10)
	seedDur(t, ctx, st, owner, "a2", "mid media", domain.KindAudio, domain.KeyAudioDuration, 60)
	seedDur(t, ctx, st, owner, "a3", "clip media", domain.KindVideo, domain.KeyVideoDuration, 120)
	seedDur(t, ctx, st, owner, "a4", "long media", domain.KindVideo, domain.KeyVideoDuration, 300)
	seedDur(t, ctx, st, owner, "a5", "photo media", domain.KindImage, "", 0) // no duration

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"minDuration", "minDuration=60", 3},
		{"maxDuration", "maxDuration=60", 2},
		{"duration band", "minDuration=30&maxDuration=200", 2},
		{"fractional minDuration", "minDuration=59.5", 3},
		{"audio+video both hit", "minDuration=60&maxDuration=120", 2},
		{"no match", "minDuration=1000", 0},
		{"max excludes durationless image", "maxDuration=100000", 4},
		{"kind+duration combo", "kind=video&minDuration=150", 1},
		{"neg minDuration unbounded", "minDuration=-5", 5},
		{"maxDuration<minDuration ignores max", "maxDuration=30&minDuration=100", 2},
	}
	for _, p := range facetPaths {
		for _, tc := range cases {
			t.Run(p.name+"/"+tc.name, func(t *testing.T) {
				url := srv.URL + p.base + "?" + p.pre + tc.query
				if got := getItems(t, url); len(got) != tc.want {
					t.Errorf("GET %s = %d assets, want %d", url, len(got), tc.want)
				}
			})
		}
	}

	// A duration band spanning the audio/video divide returns exactly one of
	// each kind, proving both duration keys are ranged over on the API path.
	kinds := map[string]int{}
	for _, a := range getItems(t, srv.URL+"/api/dam/assets?minDuration=60&maxDuration=120") {
		kinds[a.Kind]++
	}
	if kinds["audio"] != 1 || kinds["video"] != 1 {
		t.Errorf("duration band kinds = %+v, want audio:1 video:1", kinds)
	}
}

// TestTimeFilterAPI exercises ?createdAfter=/?createdBefore=/?indexedAfter=/
// ?indexedBefore= on both /assets and /search?q=, their composition, a rating
// combo, and the normalizeRange edge cases (issue #53).
func TestTimeFilterAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()

	seedTimeAPI(t, ctx, st, owner, "a1", "jan media", 100, 1000)
	feb := seedTimeAPI(t, ctx, st, owner, "a2", "feb media", 200, 2000)
	seedTimeAPI(t, ctx, st, owner, "a3", "mar media", 300, 3000)

	// Rate feb so a rating x time composition case runs.
	if err := st.BatchUpdateRating(ctx, owner, []domain.AssetID{feb}, 5); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"createdAfter", "createdAfter=200", 2},
		{"createdBefore", "createdBefore=200", 2},
		{"created band", "createdAfter=150&createdBefore=250", 1},
		{"indexedAfter", "indexedAfter=2000", 2},
		{"indexedBefore", "indexedBefore=2000", 2},
		{"created+indexed combo", "createdAfter=200&indexedBefore=2000", 1},
		{"rating+time combo", "rating=5&createdAfter=150", 1},
		{"neg createdAfter unbounded", "createdAfter=-5", 3},
		{"createdBefore<createdAfter ignores before", "createdBefore=100&createdAfter=250", 1},
		{"no match", "createdAfter=99999", 0},
	}
	for _, p := range facetPaths {
		for _, tc := range cases {
			t.Run(p.name+"/"+tc.name, func(t *testing.T) {
				url := srv.URL + p.base + "?" + p.pre + tc.query
				if got := getItems(t, url); len(got) != tc.want {
					t.Errorf("GET %s = %d assets, want %d", url, len(got), tc.want)
				}
			})
		}
	}
}
