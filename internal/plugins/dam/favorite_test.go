package dam_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// favoriteItem is the subset of the asset DTO the favorite tests assert on.
type favoriteItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Favorite bool   `json:"favorite"`
}

// TestFavoriteAPI covers the issue #62 favorite feature end to end over HTTP:
// the /batch/favorite toggle (set + clear, single and multi), the ?favorite=true
// list filter, the same filter on the FTS /search path, and that omitting the
// param leaves the pre-existing behavior untouched.
func TestFavoriteAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	mk := func(id, name string) domain.AssetID {
		aid, err := domain.NewAssetID(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpsertAsset(ctx, domain.Asset{
			ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
			StoragePath: name, Name: name, Ext: "png", Size: 1, Hash: "h-" + id,
			Width: 1, Height: 1, CreatedAt: now, IndexedAt: now,
		}); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
		return aid
	}
	a1 := mk("a1", "sunset one.png")
	a2 := mk("a2", "sunset two.png")
	_ = mk("a3", "three.png")

	list := func(query string) []favoriteItem {
		return getFavorites(t, srv.URL+"/api/dam/assets"+query)
	}

	// Fresh assets are not favorited, and the field is present in the DTO.
	all := list("")
	if len(all) != 3 {
		t.Fatalf("no filter = %d, want 3", len(all))
	}
	for _, a := range all {
		if a.Favorite {
			t.Errorf("asset %s favorited on insert, want false", a.Name)
		}
	}

	// Batch-favorite two assets (issue #62 batch action).
	postFavorite(t, srv.URL, []string{a1.String(), a2.String()}, true)

	// ?favorite=true narrows to exactly the two favorited assets.
	fav := list("?favorite=true")
	if len(fav) != 2 {
		t.Fatalf("favorite=true = %d, want 2", len(fav))
	}
	for _, a := range fav {
		if !a.Favorite {
			t.Errorf("favorite=true returned non-favorite %s", a.Name)
		}
	}

	// Omitting the param is unchanged: still all three.
	if got := list(""); len(got) != 3 {
		t.Errorf("no filter after favorite = %d, want 3", len(got))
	}

	// The single-asset toggle is the batch endpoint with one id; unfavorite a1.
	postFavorite(t, srv.URL, []string{a1.String()}, false)
	fav = list("?favorite=true")
	if len(fav) != 1 || fav[0].ID != a2.String() {
		t.Errorf("favorite=true after unfavorite a1 = %+v, want [a2]", fav)
	}

	// The favorite filter composes with the FTS search path identically: both
	// a1 and a2 match "sunset", but only a2 is still favorited.
	search := getFavorites(t, srv.URL+"/api/dam/search?q=sunset&favorite=true")
	if len(search) != 1 || search[0].ID != a2.String() {
		t.Errorf("search sunset&favorite=true = %+v, want [a2]", search)
	}
	// Without the param the search returns both sunset assets.
	if got := getFavorites(t, srv.URL+"/api/dam/search?q=sunset"); len(got) != 2 {
		t.Errorf("search sunset = %d, want 2", len(got))
	}
}

// getFavorites GETs url and decodes the asset list, failing on a non-200.
func getFavorites(t *testing.T, url string) []favoriteItem {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", url, resp.StatusCode)
	}
	var out []favoriteItem
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return out
}

// postFavorite POSTs a /batch/favorite request and asserts a 200.
func postFavorite(t *testing.T, base string, ids []string, favorite bool) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"asset_ids": ids, "favorite": favorite})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(base+"/api/dam/batch/favorite", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /batch/favorite: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /batch/favorite status = %d, want 200", resp.StatusCode)
	}
}
