package dam_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// seedKind upserts a live asset of the given kind, with a searchable name.
func seedKind(t *testing.T, ctx context.Context, st interface {
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
		t.Fatalf("upsert %s: %v", id, err)
	}
	return aid
}

func getItems(t *testing.T, url string) []assetItem {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", url, resp.StatusCode)
	}
	var out []assetItem
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return out
}

// TestListAssetsKindFilterAPI covers ?kind= (single + multi-value) on /assets and
// its composition with ?rating= and ?tag=, per issue #75.
func TestListAssetsKindFilterAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()

	img := seedKind(t, ctx, st, owner, "a1", "one.png", domain.KindImage)
	vid := seedKind(t, ctx, st, owner, "a2", "two.mp4", domain.KindVideo)
	_ = seedKind(t, ctx, st, owner, "a3", "three.mp3", domain.KindAudio)
	_ = seedKind(t, ctx, st, owner, "a4", "four.png", domain.KindImage)

	// Rate the video 5 stars and tag it so combo filters are exercised.
	if err := st.BatchUpdateRating(ctx, owner, []domain.AssetID{vid}, 5); err != nil {
		t.Fatal(err)
	}
	tid, err := domain.NewTagID("t1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateTag(ctx, domain.Tag{ID: tid, Owner: owner, Name: "hero"}); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchAddTags(ctx, owner, []domain.AssetID{img, vid}, []domain.TagID{tid}); err != nil {
		t.Fatal(err)
	}

	base := srv.URL + "/api/dam/assets"
	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"no kind", "", 4},
		{"kind=image", "?kind=image", 2},
		{"kind=video", "?kind=video", 1},
		{"kind=image,video multi", "?kind=image,video", 3},
		{"kind unknown ignored", "?kind=bogus", 4},
		{"kind valid+unknown keeps valid", "?kind=image,bogus", 2},
		{"kind+rating combo", "?kind=video&rating=5", 1},
		{"kind+rating no match", "?kind=image&rating=5", 0},
		{"kind+rating+tag combo", "?kind=video&rating=5&tag=t1", 1},
		{"kind+tag excludes untagged", "?kind=image&tag=t1", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := getItems(t, base+tc.query); len(got) != tc.want {
				t.Errorf("GET %s = %d assets, want %d", tc.query, len(got), tc.want)
			}
		})
	}

	// Multi-value kind results carry only the requested kinds.
	for _, a := range getItems(t, base+"?kind=image,video") {
		if a.Kind != "image" && a.Kind != "video" {
			t.Errorf("kind=image,video returned kind %q", a.Kind)
		}
	}
}

// TestFacetsAPI covers GET /facets: every kind is present in display order with
// live counts, narrowed by the same folder/tag/rating context as /assets.
func TestFacetsAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()

	seedKind(t, ctx, st, owner, "a1", "one.png", domain.KindImage)
	seedKind(t, ctx, st, owner, "a2", "two.png", domain.KindImage)
	vid := seedKind(t, ctx, st, owner, "a3", "three.mp4", domain.KindVideo)
	if err := st.BatchUpdateRating(ctx, owner, []domain.AssetID{vid}, 4); err != nil {
		t.Fatal(err)
	}

	type kindCount struct {
		Kind  string `json:"kind"`
		Count int    `json:"count"`
	}
	type facets struct {
		Kinds []kindCount `json:"kinds"`
	}
	get := func(query string) map[string]int {
		resp, err := http.Get(srv.URL + "/api/dam/facets" + query)
		if err != nil {
			t.Fatalf("GET facets%s: %v", query, err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET facets%s status = %d, want 200", query, resp.StatusCode)
		}
		var f facets
		if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
			t.Fatalf("decode facets: %v", err)
		}
		// Every AssetKind must be present so the UI facet is stable.
		if len(f.Kinds) != len(domain.AllKinds) {
			t.Fatalf("facets returned %d kinds, want %d", len(f.Kinds), len(domain.AllKinds))
		}
		m := make(map[string]int, len(f.Kinds))
		for _, kc := range f.Kinds {
			m[kc.Kind] = kc.Count
		}
		return m
	}

	all := get("")
	if all["image"] != 2 || all["video"] != 1 || all["audio"] != 0 {
		t.Errorf("facets counts = %+v, want image:2 video:1 audio:0", all)
	}
	// The rating context narrows counts but still lists every kind (kind facet
	// itself is ignored so all formats stay selectable).
	rated := get("?rating=4")
	if rated["image"] != 0 || rated["video"] != 1 {
		t.Errorf("facets?rating=4 = %+v, want image:0 video:1", rated)
	}
}

// TestSearchKindFilterAPI covers ?q= composing with the ?kind= facet: keyword
// search narrows to matching assets, then the format facet narrows further.
func TestSearchKindFilterAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()

	// Three assets share the "sunset" keyword across different kinds.
	seedKind(t, ctx, st, owner, "a1", "sunset beach", domain.KindImage)
	seedKind(t, ctx, st, owner, "a2", "sunset clip", domain.KindVideo)
	seedKind(t, ctx, st, owner, "a3", "sunrise beach", domain.KindImage)

	base := srv.URL + "/api/dam/search"
	if got := getItems(t, base+"?q=sunset"); len(got) != 2 {
		t.Errorf("q=sunset = %d, want 2", len(got))
	}
	got := getItems(t, base+"?q=sunset&kind=image")
	if len(got) != 1 || got[0].Kind != "image" {
		t.Errorf("q=sunset&kind=image = %+v, want 1 image", got)
	}
	if got := getItems(t, base+"?q=sunset&kind=video"); len(got) != 1 || got[0].Kind != "video" {
		t.Errorf("q=sunset&kind=video = %+v, want 1 video", got)
	}
	if got := getItems(t, base+"?q=sunset&kind=audio"); len(got) != 0 {
		t.Errorf("q=sunset&kind=audio = %d, want 0", len(got))
	}
}
