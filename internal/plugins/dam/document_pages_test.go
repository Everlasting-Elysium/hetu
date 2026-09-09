package dam_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

type pageDTO struct {
	PageNo   int    `json:"page_no"`
	ThumbURL string `json:"thumb_url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

// seedDoc upserts a document asset (provider "local", the given storage path)
// owned by owner and returns its id, so ReplaceDocumentPages can resolve it by
// natural key.
func seedDoc(t *testing.T, st kernel.Store, owner domain.OwnerID, id, path string) domain.AssetID {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(context.Background(), domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindDocument, Provider: "local",
		StoragePath: path, Name: id, Ext: "pdf", Size: 1, Hash: id,
		CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert doc: %v", err)
	}
	return aid
}

// writeThumb writes a fake page thumbnail file and returns its absolute path;
// servePageThumb streams whatever path the page row stores.
func writeThumb(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write thumb: %v", err)
	}
	return path
}

func TestListAssetPages(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()

	aid := seedDoc(t, st, owner, "doc1", "doc1.pdf")
	p1 := writeThumb(t, "doc1_p1.jpg", []byte{0xFF, 0xD8, 0xFF, 1})
	p2 := writeThumb(t, "doc1_p2.jpg", []byte{0xFF, 0xD8, 0xFF, 2})
	if err := st.ReplaceDocumentPages(ctx, owner, "local", "doc1.pdf", []domain.DocumentPage{
		{PageNo: 1, ThumbPath: p1, Width: 800, Height: 600},
		{PageNo: 2, ThumbPath: p2, Width: 800, Height: 600},
	}); err != nil {
		t.Fatalf("replace pages: %v", err)
	}

	pages := getPages(t, srv.URL+"/api/dam/assets/doc1/pages", http.StatusOK)
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(pages))
	}
	if pages[0].PageNo != 1 || pages[1].PageNo != 2 {
		t.Errorf("page order = %d,%d, want 1,2", pages[0].PageNo, pages[1].PageNo)
	}
	wantURL := "/api/dam/assets/" + aid.String() + "/pages/1/thumb"
	if pages[0].ThumbURL != wantURL {
		t.Errorf("thumb_url = %q, want %q", pages[0].ThumbURL, wantURL)
	}
	if pages[0].Width != 800 || pages[0].Height != 600 {
		t.Errorf("dims = %dx%d, want 800x600", pages[0].Width, pages[0].Height)
	}
}

func TestServePageThumb(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()
	seedDoc(t, st, owner, "doc1", "doc1.pdf")
	body := []byte{0xFF, 0xD8, 0xFF, 42}
	p1 := writeThumb(t, "doc1_p1.jpg", body)
	if err := st.ReplaceDocumentPages(ctx, owner, "local", "doc1.pdf", []domain.DocumentPage{
		{PageNo: 1, ThumbPath: p1},
	}); err != nil {
		t.Fatalf("replace pages: %v", err)
	}

	resp, err := http.Get(srv.URL + "/api/dam/assets/doc1/pages/1/thumb")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("content-type = %q, want image/jpeg", ct)
	}
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, body) {
		t.Errorf("body = %v, want %v", got, body)
	}

	assertStatus(t, srv.URL+"/api/dam/assets/doc1/pages/9/thumb", http.StatusNotFound)
	assertStatus(t, srv.URL+"/api/dam/assets/doc1/pages/abc/thumb", http.StatusBadRequest)
}

// TestAssetPagesNotFound covers a non-existent asset on both endpoints.
func TestAssetPagesNotFound(t *testing.T) {
	srv, _, _ := newTestServer(t)
	assertStatus(t, srv.URL+"/api/dam/assets/ghost/pages", http.StatusNotFound)
	assertStatus(t, srv.URL+"/api/dam/assets/ghost/pages/1/thumb", http.StatusNotFound)
}

// TestAssetPagesEmpty asserts a document with no page rows returns 200 with [].
func TestAssetPagesEmpty(t *testing.T) {
	srv, owner, st := newTestServer(t)
	seedDoc(t, st, owner, "single", "single.pdf")
	pages := getPages(t, srv.URL+"/api/dam/assets/single/pages", http.StatusOK)
	if len(pages) != 0 {
		t.Errorf("pages = %d, want 0", len(pages))
	}
}

// TestAssetPagesCrossOwnerIsolation asserts one owner cannot read another
// owner's document pages or page thumbnails (IDOR): both return 404.
func TestAssetPagesCrossOwnerIsolation(t *testing.T) {
	srv, _, st := newTestServer(t)
	ctx := context.Background()

	other, err := domain.NewOwnerID("intruder-target")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, other); err != nil {
		t.Fatalf("ensure other owner: %v", err)
	}
	seedDoc(t, st, other, "secret", "secret.pdf")
	p1 := writeThumb(t, "secret_p1.jpg", []byte{0xFF, 0xD8, 0xFF})
	if err := st.ReplaceDocumentPages(ctx, other, "local", "secret.pdf", []domain.DocumentPage{
		{PageNo: 1, ThumbPath: p1},
	}); err != nil {
		t.Fatalf("replace pages: %v", err)
	}

	assertStatus(t, srv.URL+"/api/dam/assets/secret/pages", http.StatusNotFound)
	assertStatus(t, srv.URL+"/api/dam/assets/secret/pages/1/thumb", http.StatusNotFound)
}

func getPages(t *testing.T, url string, wantStatus int) []pageDTO {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, wantStatus)
	}
	var out []pageDTO
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func assertStatus(t *testing.T, url string, want int) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, want)
	}
}
