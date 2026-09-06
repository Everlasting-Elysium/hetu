package dam_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// doReq issues a request with an optional JSON body and returns status + body.
func doReq(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// noteOf lists assets and returns the note field for the given asset id.
func noteOf(t *testing.T, srvURL, assetID string) string {
	t.Helper()
	status, body := doReq(t, http.MethodGet, srvURL+"/api/dam/assets", "")
	if status != http.StatusOK {
		t.Fatalf("list assets status = %d", status)
	}
	var assets []struct {
		ID   string `json:"id"`
		Note string `json:"note"`
	}
	if err := json.Unmarshal(body, &assets); err != nil {
		t.Fatalf("decode assets: %v", err)
	}
	for _, a := range assets {
		if a.ID == assetID {
			return a.Note
		}
	}
	t.Fatalf("asset %s not found in list", assetID)
	return ""
}

// TestNoteAPI_lifecycle exercises the four acceptance criteria end-to-end:
// PUT sets a note readable via GET; the note is full-text searchable; DELETE
// removes it and clears FTS.
func TestNoteAPI_lifecycle(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	aid, err := domain.NewAssetID("a1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: "cat.png", Name: "cat", Ext: "png",
		Size: 1, Hash: "h1", Width: 1, Height: 1, CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// PUT a note.
	status, body := doReq(t, http.MethodPut, srv.URL+"/api/dam/assets/a1/note", `{"text":"lovely sunset panorama"}`)
	if status != http.StatusOK {
		t.Fatalf("PUT note status = %d, body = %s", status, body)
	}

	// GET the asset list reflects the note.
	if got := noteOf(t, srv.URL, "a1"); got != "lovely sunset panorama" {
		t.Errorf("note = %q, want %q", got, "lovely sunset panorama")
	}

	// The note text is full-text searchable.
	got := getSearch(t, srv.URL+"/api/dam/search?q=panorama", http.StatusOK)
	if len(got) != 1 || got[0].ID != "a1" {
		t.Errorf("search panorama = %+v, want 1 match (a1)", got)
	}

	// DELETE the note.
	status, body = doReq(t, http.MethodDelete, srv.URL+"/api/dam/assets/a1/note", "")
	if status != http.StatusOK {
		t.Fatalf("DELETE note status = %d, body = %s", status, body)
	}

	// GET shows the note cleared.
	if got := noteOf(t, srv.URL, "a1"); got != "" {
		t.Errorf("note after delete = %q, want empty", got)
	}
	// Search no longer hits.
	if got := getSearch(t, srv.URL+"/api/dam/search?q=panorama", http.StatusOK); len(got) != 0 {
		t.Errorf("search panorama after delete = %d results, want 0", len(got))
	}
}

// TestNoteAPI_whitespaceRejected proves a blank/whitespace-only note is a 400.
func TestNoteAPI_whitespaceRejected(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	aid, _ := domain.NewAssetID("a1")
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: "cat.png", Name: "cat", Ext: "png",
		Size: 1, Hash: "h1", CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	for _, tc := range []string{`{"text":""}`, `{"text":"   "}`, `{"text":"\n\t"}`} {
		status, _ := doReq(t, http.MethodPut, srv.URL+"/api/dam/assets/a1/note", tc)
		if status != http.StatusBadRequest {
			t.Errorf("PUT %s status = %d, want 400", tc, status)
		}
	}
}

// TestNoteAPI_trimsWhitespace proves surrounding whitespace is trimmed before
// storage, so the persisted note is the clean value.
func TestNoteAPI_trimsWhitespace(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	aid, _ := domain.NewAssetID("a1")
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: "cat.png", Name: "cat", Ext: "png",
		Size: 1, Hash: "h1", CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	status, _ := doReq(t, http.MethodPut, srv.URL+"/api/dam/assets/a1/note", `{"text":"  padded note  "}`)
	if status != http.StatusOK {
		t.Fatalf("PUT note status = %d", status)
	}
	if got := noteOf(t, srv.URL, "a1"); got != "padded note" {
		t.Errorf("note = %q, want %q (trimmed)", got, "padded note")
	}
}

// TestNoteAPI_missingAsset proves a note write to an unknown asset is a 404.
func TestNoteAPI_missingAsset(t *testing.T) {
	srv, _, _ := newTestServer(t)
	status, _ := doReq(t, http.MethodPut, srv.URL+"/api/dam/assets/ghost/note", `{"text":"hi"}`)
	if status != http.StatusNotFound {
		t.Errorf("PUT note on missing asset status = %d, want 404", status)
	}
}
