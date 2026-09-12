package dam_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type tagResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	ParentID string `json:"parent_id"`
}

// createTag POSTs a tag and returns it.
func createTag(t *testing.T, base string, body map[string]string) tagResult {
	t.Helper()
	resp := collDo(t, http.MethodPost, base+"/tags", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tag status = %d, want 201", resp.StatusCode)
	}
	var tg tagResult
	if err := json.NewDecoder(resp.Body).Decode(&tg); err != nil {
		t.Fatal(err)
	}
	return tg
}

// assetTagNames GETs one asset's tag names.
func assetTagNames(t *testing.T, base, assetID string) []string {
	t.Helper()
	resp := collDo(t, http.MethodGet, base+"/assets/"+assetID+"/tags", nil)
	defer resp.Body.Close()
	var tags []tagResult
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(tags))
	for i, tg := range tags {
		names[i] = tg.Name
	}
	return names
}

func TestMergeTagsAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	seedTestAsset(t, ctx, st, owner, "a1", "1.png")
	seedTestAsset(t, ctx, st, owner, "a2", "2.png")
	base := srv.URL + "/api/dam"

	from := createTag(t, base, map[string]string{"name": "from"})
	into := createTag(t, base, map[string]string{"name": "into"})

	// a1 has from only; a2 has both from and into (dedup case).
	collDo(t, http.MethodPost, base+"/batch/tag",
		map[string]any{"asset_ids": []string{"a1", "a2"}, "tag_ids": []string{from.ID}}).Body.Close()
	collDo(t, http.MethodPost, base+"/batch/tag",
		map[string]any{"asset_ids": []string{"a2"}, "tag_ids": []string{into.ID}}).Body.Close()

	resp := collDo(t, http.MethodPost, base+"/tags/merge",
		map[string]string{"from_tag_id": from.ID, "to_tag_id": into.ID})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merge status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// The source tag is gone from the global list.
	resp = collDo(t, http.MethodGet, base+"/tags", nil)
	var tags []tagResult
	json.NewDecoder(resp.Body).Decode(&tags)
	resp.Body.Close()
	if len(tags) != 1 || tags[0].Name != "into" {
		t.Fatalf("tags after merge = %+v, want [into]", tags)
	}
	// Both assets now carry a single into tag.
	for _, id := range []string{"a1", "a2"} {
		if names := assetTagNames(t, base, id); len(names) != 1 || names[0] != "into" {
			t.Fatalf("%s tags after merge = %v, want [into]", id, names)
		}
	}
}

func TestMergeTagsAPI_errors(t *testing.T) {
	srv, _, _ := newTestServer(t)
	base := srv.URL + "/api/dam"
	tag := createTag(t, base, map[string]string{"name": "solo"})

	// Merge into self -> 400.
	resp := collDo(t, http.MethodPost, base+"/tags/merge",
		map[string]string{"from_tag_id": tag.ID, "to_tag_id": tag.ID})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("self merge status = %d, want 400", resp.StatusCode)
	}
	// Unknown source -> 404.
	resp = collDo(t, http.MethodPost, base+"/tags/merge",
		map[string]string{"from_tag_id": "ghost", "to_tag_id": tag.ID})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown source status = %d, want 404", resp.StatusCode)
	}
	// Empty id -> 400 (unparseable).
	resp = collDo(t, http.MethodPost, base+"/tags/merge",
		map[string]string{"from_tag_id": "", "to_tag_id": tag.ID})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty id status = %d, want 400", resp.StatusCode)
	}
}

func TestBatchReplaceTagAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	seedTestAsset(t, ctx, st, owner, "a1", "1.png")
	seedTestAsset(t, ctx, st, owner, "a2", "2.png") // NOT in the replace subset
	base := srv.URL + "/api/dam"

	from := createTag(t, base, map[string]string{"name": "from"})
	into := createTag(t, base, map[string]string{"name": "into"})

	// Both assets carry "from".
	collDo(t, http.MethodPost, base+"/batch/tag",
		map[string]any{"asset_ids": []string{"a1", "a2"}, "tag_ids": []string{from.ID}}).Body.Close()

	// Replace only on a1.
	resp := collDo(t, http.MethodPost, base+"/batch/replace-tag",
		map[string]any{"asset_ids": []string{"a1"}, "from_tag_id": from.ID, "to_tag_id": into.ID})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replace status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// a1 swapped; a2 unaffected.
	if names := assetTagNames(t, base, "a1"); len(names) != 1 || names[0] != "into" {
		t.Fatalf("a1 tags = %v, want [into]", names)
	}
	if names := assetTagNames(t, base, "a2"); len(names) != 1 || names[0] != "from" {
		t.Fatalf("a2 tags = %v, want [from] (unaffected)", names)
	}
	// The source tag survives (a2 still uses it).
	resp = collDo(t, http.MethodGet, base+"/tags", nil)
	var tags []tagResult
	json.NewDecoder(resp.Body).Decode(&tags)
	resp.Body.Close()
	if len(tags) != 2 {
		t.Fatalf("tags after replace = %+v, want both from+into to survive", tags)
	}

	// Replace with same tag -> 400.
	resp = collDo(t, http.MethodPost, base+"/batch/replace-tag",
		map[string]any{"asset_ids": []string{"a2"}, "from_tag_id": from.ID, "to_tag_id": from.ID})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("same-tag replace status = %d, want 400", resp.StatusCode)
	}
}
