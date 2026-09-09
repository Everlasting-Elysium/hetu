package dam_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

type collectionResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Cover    string `json:"cover"`
}

type collectionItemResult struct {
	AssetID    string `json:"asset_id"`
	Ord        int    `json:"ord"`
	AssetKind  string `json:"asset_kind"`
	AssetName  string `json:"asset_name"`
	AssetThumb string `json:"asset_thumb"`
}

// collDo issues a request with an optional JSON body and returns the response;
// the caller closes the body.
func collDo(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// fetchCollections GETs the collection list and decodes it.
func fetchCollections(t *testing.T, url string) []collectionResult {
	t.Helper()
	resp := collDo(t, http.MethodGet, url, nil)
	defer resp.Body.Close()
	var out []collectionResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCollectionCRUD(t *testing.T) {
	srv, _, _ := newTestServer(t)
	base := srv.URL + "/api/dam/collections"

	resp := collDo(t, http.MethodPost, base, map[string]string{"name": "Trip"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	var created collectionResult
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.Name != "Trip" || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}

	// Empty name is a client error.
	resp = collDo(t, http.MethodPost, base, map[string]string{"name": ""})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name status = %d, want 400", resp.StatusCode)
	}

	if got := fetchCollections(t, base); len(got) != 1 || got[0].Name != "Trip" {
		t.Fatalf("list = %+v, want [Trip]", got)
	}

	// Rename via PATCH (partial update).
	resp = collDo(t, http.MethodPatch, base+"/"+created.ID, map[string]string{"name": "Vacation"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d, want 200", resp.StatusCode)
	}
	if got := fetchCollections(t, base); len(got) != 1 || got[0].Name != "Vacation" {
		t.Fatalf("after rename = %+v, want [Vacation]", got)
	}

	// PATCH a missing collection → 404.
	resp = collDo(t, http.MethodPatch, base+"/ghost", map[string]string{"name": "X"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("patch missing status = %d, want 404", resp.StatusCode)
	}

	resp = collDo(t, http.MethodDelete, base+"/"+created.ID, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", resp.StatusCode)
	}
	if got := fetchCollections(t, base); len(got) != 0 {
		t.Fatalf("after delete = %d, want 0", len(got))
	}
}

func TestCollectionItemsAndReorder(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	seedTestVideoAsset(t, ctx, st, owner, "v1", "clip.mp4")
	seedTestAsset(t, ctx, st, owner, "a2", "b.png")
	seedTestAsset(t, ctx, st, owner, "a3", "c.png")

	resp := collDo(t, http.MethodPost, srv.URL+"/api/dam/collections", map[string]string{"name": "C"})
	var c collectionResult
	json.NewDecoder(resp.Body).Decode(&c)
	resp.Body.Close()
	itemsURL := srv.URL + "/api/dam/collections/" + c.ID + "/items"

	// An unparseable asset_id is a client error (collection exists).
	bad := collDo(t, http.MethodPost, itemsURL, map[string]string{"asset_id": ""})
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty asset_id status = %d, want 400", bad.StatusCode)
	}

	for _, id := range []string{"v1", "a2", "a3"} {
		r := collDo(t, http.MethodPost, itemsURL, map[string]string{"asset_id": id})
		if r.StatusCode != http.StatusCreated {
			t.Fatalf("add %s status = %d, want 201", id, r.StatusCode)
		}
		r.Body.Close()
	}

	items := fetchItems(t, itemsURL)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	// First member is enriched from the asset row (issue #86 parity).
	if items[0].AssetID != "v1" || items[0].AssetKind != "video" ||
		items[0].AssetName != "clip.mp4" || items[0].AssetThumb != "thumbs/v1.jpg" {
		t.Fatalf("item0 = %+v, want enriched v1 video", items[0])
	}

	// Reorder to a3, a2, v1.
	resp = collDo(t, http.MethodPut, itemsURL+"/order", map[string][]string{"asset_ids": {"a3", "a2", "v1"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reorder status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
	items = fetchItems(t, itemsURL)
	if items[0].AssetID != "a3" || items[1].AssetID != "a2" || items[2].AssetID != "v1" {
		t.Fatalf("reordered = %+v, want a3,a2,v1", items)
	}

	// A reorder set that does not match the members exactly → 400.
	resp = collDo(t, http.MethodPut, itemsURL+"/order", map[string][]string{"asset_ids": {"a3", "a2"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reorder mismatch status = %d, want 400", resp.StatusCode)
	}

	// Remove a member.
	resp = collDo(t, http.MethodDelete, itemsURL+"/v1", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove status = %d, want 200", resp.StatusCode)
	}
	if got := fetchItems(t, itemsURL); len(got) != 2 {
		t.Fatalf("after remove = %d, want 2", len(got))
	}
}

func TestCollectionCoverViaAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	seedTestAsset(t, ctx, st, owner, "a1", "1.png")
	seedTestAsset(t, ctx, st, owner, "a2", "2.png")

	base := srv.URL + "/api/dam/collections"
	resp := collDo(t, http.MethodPost, base, map[string]string{"name": "C"})
	var c collectionResult
	json.NewDecoder(resp.Body).Decode(&c)
	resp.Body.Close()
	itemsURL := base + "/" + c.ID + "/items"
	collDo(t, http.MethodPost, itemsURL, map[string]string{"asset_id": "a1"}).Body.Close()
	collDo(t, http.MethodPost, itemsURL, map[string]string{"asset_id": "a2"}).Body.Close()

	// No override: effective cover is the lowest-ord member (a1).
	if got := fetchCollections(t, base); got[0].Cover != "a1" {
		t.Fatalf("fallback cover = %q, want a1", got[0].Cover)
	}

	// Override to a member.
	resp = collDo(t, http.MethodPatch, base+"/"+c.ID, map[string]string{"cover": "a2"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set cover status = %d, want 200", resp.StatusCode)
	}
	if got := fetchCollections(t, base); got[0].Cover != "a2" {
		t.Fatalf("override cover = %q, want a2", got[0].Cover)
	}

	// A cover that is not a member → 404.
	seedTestAsset(t, ctx, st, owner, "a9", "9.png")
	resp = collDo(t, http.MethodPatch, base+"/"+c.ID, map[string]string{"cover": "a9"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("non-member cover status = %d, want 404", resp.StatusCode)
	}
}

// TestCollectionParentCycleAPI proves PATCH rejects a parent_id that would make
// a collection its own ancestor, mapped to 400 (not 500).
func TestCollectionParentCycleAPI(t *testing.T) {
	srv, _, _ := newTestServer(t)
	base := srv.URL + "/api/dam/collections"

	var a, b collectionResult
	resp := collDo(t, http.MethodPost, base, map[string]string{"name": "A"})
	json.NewDecoder(resp.Body).Decode(&a)
	resp.Body.Close()
	resp = collDo(t, http.MethodPost, base, map[string]string{"name": "B", "parent_id": a.ID})
	json.NewDecoder(resp.Body).Decode(&b)
	resp.Body.Close()

	// Self-parent.
	resp = collDo(t, http.MethodPatch, base+"/"+a.ID, map[string]string{"parent_id": a.ID})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("self-parent status = %d, want 400", resp.StatusCode)
	}
	// Cycle: reparent a (b's parent) under b.
	resp = collDo(t, http.MethodPatch, base+"/"+a.ID, map[string]string{"parent_id": b.ID})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("cycle status = %d, want 400", resp.StatusCode)
	}
	// A non-existent parent is 404, not a 500.
	resp = collDo(t, http.MethodPatch, base+"/"+a.ID, map[string]string{"parent_id": "ghost"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing parent status = %d, want 404", resp.StatusCode)
	}
}

// TestAddCollectionItem_otherOwnerAssetAPI proves the HTTP path also rejects
// attaching an asset owned by a different owner (404, not leaked as a 201).
func TestAddCollectionItem_otherOwnerAssetAPI(t *testing.T) {
	srv, _, st := newTestServer(t)
	ctx := t.Context()
	other, err := domain.NewOwnerID("intruder")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, other); err != nil {
		t.Fatalf("ensure other owner: %v", err)
	}
	seedTestAsset(t, ctx, st, other, "theirs", "secret.png")

	base := srv.URL + "/api/dam/collections"
	resp := collDo(t, http.MethodPost, base, map[string]string{"name": "C"})
	var c collectionResult
	json.NewDecoder(resp.Body).Decode(&c)
	resp.Body.Close()

	resp = collDo(t, http.MethodPost, base+"/"+c.ID+"/items", map[string]string{"asset_id": "theirs"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("add other-owner asset status = %d, want 404", resp.StatusCode)
	}
	if got := fetchItems(t, base+"/"+c.ID+"/items"); len(got) != 0 {
		t.Fatalf("items after rejected add = %+v, want none", got)
	}
}

func TestCollectionItemsNotFound(t *testing.T) {
	srv, _, _ := newTestServer(t)
	itemsURL := srv.URL + "/api/dam/collections/ghost/items"

	resp := collDo(t, http.MethodGet, itemsURL, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("list items status = %d, want 404", resp.StatusCode)
	}
	resp = collDo(t, http.MethodPost, itemsURL, map[string]string{"asset_id": "a1"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("add item status = %d, want 404", resp.StatusCode)
	}
}

// fetchItems GETs a collection's items and decodes them.
func fetchItems(t *testing.T, url string) []collectionItemResult {
	t.Helper()
	resp := collDo(t, http.MethodGet, url, nil)
	defer resp.Body.Close()
	var out []collectionItemResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
