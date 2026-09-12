package dam_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

type folderResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Path     string `json:"path"`
	Cover    string `json:"cover"`
	CoverURL string `json:"cover_url"`
	Color    string `json:"color"`
}

// seedFolderAsset upserts a live image asset assigned to folderID with a
// thumbnail and an explicit indexed_at so cover-fallback ordering can be tested.
func seedFolderAsset(t *testing.T, ctx context.Context, st kernel.Store, owner domain.OwnerID, id, folderID string, indexed time.Time) {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: id + ".png", Name: id + ".png", Ext: "png", Size: 1, Hash: "h-" + id,
		ThumbPath: "thumbs/" + id + ".jpg", Width: 1, Height: 1,
		CreatedAt: indexed, IndexedAt: indexed, FolderID: folderID,
	}); err != nil {
		t.Fatalf("seed folder asset %s: %v", id, err)
	}
}

func createFolder(t *testing.T, url, name, path string) folderResult {
	t.Helper()
	resp := collDo(t, http.MethodPost, url, map[string]string{"name": name, "path": path})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create folder status = %d, want 201", resp.StatusCode)
	}
	var f folderResult
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		t.Fatal(err)
	}
	return f
}

func fetchFolders(t *testing.T, url string) []folderResult {
	t.Helper()
	resp := collDo(t, http.MethodGet, url, nil)
	defer resp.Body.Close()
	var out []folderResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestFolderCoverColorViaAPI covers the explicit cover override, the color
// label, and the resolved cover_url on the list response.
func TestFolderCoverColorViaAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	base := srv.URL + "/api/dam/folders"
	f := createFolder(t, base, "F", "folder1")
	now := time.Now().UTC().Truncate(time.Second)
	seedFolderAsset(t, ctx, st, owner, "a1", f.ID, now)

	// Set an explicit cover + color.
	resp := collDo(t, http.MethodPatch, base+"/"+f.ID, map[string]string{"cover": "a1", "color": "#e5484d"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set cover/color status = %d, want 200", resp.StatusCode)
	}

	got := fetchFolders(t, base)
	if len(got) != 1 {
		t.Fatalf("folders = %d, want 1", len(got))
	}
	if got[0].Cover != "a1" {
		t.Fatalf("cover = %q, want a1", got[0].Cover)
	}
	if got[0].Color != "#e5484d" {
		t.Fatalf("color = %q, want #e5484d", got[0].Color)
	}
	if want := "/api/dam/assets/a1/thumb"; got[0].CoverURL != want {
		t.Fatalf("cover_url = %q, want %q", got[0].CoverURL, want)
	}
}

// TestFolderCoverFallback verifies that with no explicit cover the effective
// cover is the folder's earliest-indexed live asset, and that trashing that
// asset advances the fallback to the next-earliest.
func TestFolderCoverFallback(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	base := srv.URL + "/api/dam/folders"
	f := createFolder(t, base, "F", "folder1")

	now := time.Now().UTC().Truncate(time.Second)
	// a2 is indexed earlier than a1, so a2 must win the fallback despite the id
	// order — proving the ORDER BY indexed_at ASC drives the pick.
	seedFolderAsset(t, ctx, st, owner, "a1", f.ID, now)
	seedFolderAsset(t, ctx, st, owner, "a2", f.ID, now.Add(-time.Hour))

	got := fetchFolders(t, base)
	if got[0].Cover != "a2" {
		t.Fatalf("fallback cover = %q, want a2 (earliest indexed)", got[0].Cover)
	}
	if want := "/api/dam/assets/a2/thumb"; got[0].CoverURL != want {
		t.Fatalf("fallback cover_url = %q, want %q", got[0].CoverURL, want)
	}

	// Trash a2; the fallback advances to a1 (deleted_at IS NULL filter).
	aid, err := domain.NewAssetID("a2")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.BatchTrashAssets(ctx, owner, []domain.AssetID{aid}); err != nil {
		t.Fatalf("trash a2: %v", err)
	}
	if got := fetchFolders(t, base); got[0].Cover != "a1" {
		t.Fatalf("fallback after trash = %q, want a1", got[0].Cover)
	}
}

// TestFolderCoverRejectsInvalid verifies a cover pointing at a non-existent
// asset, a trashed asset, or another owner's asset is rejected with 404.
func TestFolderCoverRejectsInvalid(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := t.Context()
	base := srv.URL + "/api/dam/folders"
	f := createFolder(t, base, "F", "folder1")

	// Non-existent asset id.
	resp := collDo(t, http.MethodPatch, base+"/"+f.ID, map[string]string{"cover": "ghost"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("nonexistent cover status = %d, want 404", resp.StatusCode)
	}

	// Trashed asset (owned but soft-deleted) must not be usable as a cover.
	now := time.Now().UTC().Truncate(time.Second)
	seedFolderAsset(t, ctx, st, owner, "dead", f.ID, now)
	dead, err := domain.NewAssetID("dead")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.BatchTrashAssets(ctx, owner, []domain.AssetID{dead}); err != nil {
		t.Fatalf("trash dead: %v", err)
	}
	resp = collDo(t, http.MethodPatch, base+"/"+f.ID, map[string]string{"cover": "dead"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("trashed cover status = %d, want 404", resp.StatusCode)
	}

	// Another owner's asset (IDOR guard): seed it under a different owner.
	other, err := domain.NewOwnerID("intruder")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, other); err != nil {
		t.Fatalf("ensure other owner: %v", err)
	}
	seedFolderAsset(t, ctx, st, other, "theirs", "", now)
	resp = collDo(t, http.MethodPatch, base+"/"+f.ID, map[string]string{"cover": "theirs"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-owner cover status = %d, want 404", resp.StatusCode)
	}
}
