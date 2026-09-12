package wallpaper

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// TestListDefaultKind covers the wallpaper-specific default: a blank ?kind=
// narrows to image+video (NOT "any format" like DAM), so audio/documents never
// surface in the gallery.
func TestListDefaultKind(t *testing.T) {
	e := newTestEnv(t)
	e.add(seed{id: "img", name: "one.png", kind: domain.KindImage})
	e.add(seed{id: "vid", name: "two.mp4", kind: domain.KindVideo})
	e.add(seed{id: "aud", name: "three.mp3", kind: domain.KindAudio})
	e.add(seed{id: "doc", name: "four.pdf", kind: domain.KindDocument})

	got := e.getDTOs("/api/wallpaper/list")
	if len(got) != 2 {
		t.Fatalf("default list = %d, want 2 (image+video only)", len(got))
	}
	for _, d := range got {
		if d.Kind != "image" && d.Kind != "video" {
			t.Errorf("default list leaked kind %q", d.Kind)
		}
	}
}

// TestListFilters covers kind/shape/minWidth/rating/collection narrowing and a
// combination, all over the wallpaper endpoint.
func TestListFilters(t *testing.T) {
	e := newTestEnv(t)
	land := e.add(seed{id: "land", name: "land.png", width: 1920, height: 1080, rating: 5})
	e.add(seed{id: "port", name: "port.png", width: 1080, height: 1920, rating: 1})
	e.add(seed{id: "sq", name: "sq.png", width: 1000, height: 1000, rating: 3})
	e.add(seed{id: "vid", name: "clip.mp4", kind: domain.KindVideo, width: 1920, height: 1080})

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"kind=image", "?kind=image", 3},
		{"shape=landscape spans image+video", "?shape=landscape", 2},
		{"kind=image shape=landscape", "?kind=image&shape=landscape", 1},
		{"minWidth", "?minWidth=1500", 2},
		{"rating>=3", "?rating=3", 2},
		{"invalid kind falls back to default", "?kind=bogus", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.getDTOs("/api/wallpaper/list" + tc.query); len(got) != tc.want {
				t.Errorf("list%s = %d, want %d", tc.query, len(got), tc.want)
			}
		})
	}

	// A collection facet narrows to its members regardless of shape/kind.
	cid := e.newCollection("c1", "picks")
	e.addToCollection(cid, land)
	if got := e.getDTOs("/api/wallpaper/list?collection=" + cid); len(got) != 1 || got[0].ID != land.String() {
		t.Errorf("collection filter = %v, want [land]", dtoIDs(got))
	}
}

// TestListSort covers the three sorts: latest (indexed DESC), rating (rating
// DESC), random (set preserved, order unchecked), and that an invalid sort
// falls back to latest rather than erroring.
func TestListSort(t *testing.T) {
	e := newTestEnv(t)
	base := time.Now().UTC().Truncate(time.Second)
	a1 := e.add(seed{id: "a1", name: "one.png", rating: 1, indexed: base})
	a2 := e.add(seed{id: "a2", name: "two.png", rating: 5, indexed: base.Add(time.Second)})
	a3 := e.add(seed{id: "a3", name: "three.png", rating: 3, indexed: base.Add(2 * time.Second)})

	latest := []string{a3.String(), a2.String(), a1.String()}
	if got := dtoIDs(e.getDTOs("/api/wallpaper/list?sort=latest")); !reflect.DeepEqual(got, latest) {
		t.Errorf("sort=latest = %v, want %v", got, latest)
	}
	if got := dtoIDs(e.getDTOs("/api/wallpaper/list?sort=bogus")); !reflect.DeepEqual(got, latest) {
		t.Errorf("sort=bogus (fallback) = %v, want latest %v", got, latest)
	}
	wantRating := []string{a2.String(), a3.String(), a1.String()}
	if got := dtoIDs(e.getDTOs("/api/wallpaper/list?sort=rating")); !reflect.DeepEqual(got, wantRating) {
		t.Errorf("sort=rating = %v, want %v", got, wantRating)
	}
	if got := e.getDTOs("/api/wallpaper/list?sort=random"); len(got) != 3 {
		t.Errorf("sort=random returned %d, want 3", len(got))
	}
}

// TestListDTOHidesInternals asserts the public DTO never carries storage_path/
// provider/folder_id/name — only the safe id/dims plus relative URLs.
func TestListDTOHidesInternals(t *testing.T) {
	e := newTestEnv(t)
	a1 := e.add(seed{id: "a1", name: "secret-path.png", width: 800, height: 600})

	resp := e.get("/api/wallpaper/list")
	defer func() { _ = resp.Body.Close() }()
	var raw []map[string]any
	decodeJSON(t, resp, &raw)
	if len(raw) != 1 {
		t.Fatalf("got %d, want 1", len(raw))
	}
	for _, banned := range []string{"storage_path", "provider", "folder_id", "name", "path"} {
		if _, ok := raw[0][banned]; ok {
			t.Errorf("DTO leaked forbidden field %q", banned)
		}
	}
	if raw[0]["thumb_url"] != "/api/wallpaper/"+a1.String()+"/thumb" {
		t.Errorf("thumb_url = %v", raw[0]["thumb_url"])
	}
}

// TestPublicNoAuth proves the read-only endpoints need no credentials: a request
// carrying no Authorization header still succeeds (the public-gallery contract).
func TestPublicNoAuth(t *testing.T) {
	e := newTestEnv(t)
	e.add(seed{id: "a1", name: "one.png"})

	req, err := http.NewRequest(http.MethodGet, e.srv.URL+"/api/wallpaper/list", nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("test bug: request unexpectedly carries an auth header")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("anonymous GET /list = %d, want 200", resp.StatusCode)
	}
}
