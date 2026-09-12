package wallpaper

import (
	"net/http"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// TestListCollections covers the collection listing and its cover URL
// resolution: an explicit cover with a thumbnail yields a cover_url, a
// collection whose cover asset has no thumbnail omits it.
func TestListCollections(t *testing.T) {
	e := newTestEnv(t)
	withThumb := e.add(seed{id: "a1", name: "one.png", thumb: []byte("THUMB")})
	noThumb := e.add(seed{id: "a2", name: "two.png"})

	c1 := e.newCollection("c1", "has-cover")
	e.addToCollection(c1, withThumb)
	if err := e.st.UpdateCollection(e.ctx, e.owner, mustCID(t, c1), "has-cover", "", withThumb.String()); err != nil {
		t.Fatal(err)
	}
	c2 := e.newCollection("c2", "cover-no-thumb")
	e.addToCollection(c2, noThumb)
	if err := e.st.UpdateCollection(e.ctx, e.owner, mustCID(t, c2), "cover-no-thumb", "", noThumb.String()); err != nil {
		t.Fatal(err)
	}

	resp := e.get("/api/wallpaper/collections")
	defer func() { _ = resp.Body.Close() }()
	var got []collectionDTO
	decodeJSON(t, resp, &got)

	byID := map[string]collectionDTO{}
	for _, c := range got {
		byID[c.ID] = c
	}
	if byID[c1].CoverURL != "/api/wallpaper/"+withThumb.String()+"/thumb" {
		t.Errorf("c1 cover_url = %q, want the thumb endpoint", byID[c1].CoverURL)
	}
	if byID[c2].CoverURL != "" {
		t.Errorf("c2 cover_url = %q, want empty (cover asset has no thumbnail)", byID[c2].CoverURL)
	}
}

// TestGetCollectionAssets covers the member listing: ord order, full dimension
// fields, and a 404 for an unknown collection. It intentionally does NOT apply
// the wallpaper facet filter (a collection is already curated).
func TestGetCollectionAssets(t *testing.T) {
	e := newTestEnv(t)
	a1 := e.add(seed{id: "a1", name: "one.png", width: 800})
	a2 := e.add(seed{id: "a2", name: "two.png", width: 1600})

	cid := e.newCollection("c1", "picks")
	e.addToCollection(cid, a2) // ord 0
	e.addToCollection(cid, a1) // ord 1

	got := e.getDTOs("/api/wallpaper/collections/" + cid)
	if len(got) != 2 || got[0].ID != a2.String() || got[1].ID != a1.String() {
		t.Fatalf("collection assets = %v, want ord [a2 a1]", dtoIDs(got))
	}
	if got[0].Width != 1600 {
		t.Errorf("a2 width = %d, want 1600 (full asset fields)", got[0].Width)
	}

	resp := e.get("/api/wallpaper/collections/does-not-exist")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing collection = %d, want 404", resp.StatusCode)
	}
}

// mustCID parses a collection id string for the update helper above.
func mustCID(t *testing.T, s string) domain.CollectionID {
	t.Helper()
	id, err := domain.NewCollectionID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
