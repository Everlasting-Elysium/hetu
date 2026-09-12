package wallpaper

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// addVersionedAsset seeds an asset whose anchor file holds anchorBody, then adds
// a current version whose (different) file holds currentBody. It exercises
// currentFile's version-resolution branch (issue #58): the anchor's storage_path
// stays put, but the current version points elsewhere, so downloads must serve
// the version's bytes — not the anchor's.
func (e *testEnv) addVersionedAsset(anchorName, anchorBody, versionName, currentBody string) domain.AssetID {
	e.t.Helper()
	aid := e.add(seed{id: "ver1", name: anchorName, content: []byte(anchorBody)})
	// The version file must exist under the same local provider root to be opened.
	if err := os.WriteFile(filepath.Join(e.libDir, versionName), []byte(currentBody), 0o644); err != nil {
		e.t.Fatal(err)
	}
	anchor, err := e.st.GetAsset(e.ctx, e.owner, aid)
	if err != nil {
		e.t.Fatal(err)
	}
	v1, err := domain.NewVersionID("ver1-v1")
	if err != nil {
		e.t.Fatal(err)
	}
	v2, err := domain.NewVersionID("ver1-v2")
	if err != nil {
		e.t.Fatal(err)
	}
	// base = the lazy v1 backfill from the anchor state; newV = the uploaded
	// current version at a different storage path (mirrors sqlite_versions_test).
	base := domain.AssetVersion{
		ID: v1, AssetID: aid, Owner: e.owner, Provider: anchor.Provider,
		StoragePath: anchor.StoragePath, Hash: anchor.Hash, Size: anchor.Size,
		Width: anchor.Width, Height: anchor.Height, CreatedAt: anchor.CreatedAt,
	}
	newV := domain.AssetVersion{
		ID: v2, AssetID: aid, Owner: e.owner, Provider: "local",
		StoragePath: versionName, Hash: "h-ver1-v2", Size: int64(len(currentBody)),
		Width: anchor.Width, Height: anchor.Height, CreatedAt: time.Now().UTC(),
	}
	if _, err := e.st.AddVersion(e.ctx, e.owner, base, newV); err != nil {
		e.t.Fatalf("add version: %v", err)
	}
	return aid
}

// TestDownloadServesCurrentVersion covers currentFile's version-aware branch:
// with a current version present, /download and /download/zip must return the
// current version's bytes ("CURRENT"), never the anchor file's ("ANCHOR").
func TestDownloadServesCurrentVersion(t *testing.T) {
	e := newTestEnv(t)
	aid := e.addVersionedAsset("anchor.png", "ANCHOR", "current.png", "CURRENT")

	resp := e.get("/api/wallpaper/" + aid.String() + "/download")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "CURRENT" {
		t.Errorf("download body = %q, want CURRENT (current version, not anchor)", body)
	}

	// The zip entry name comes from the asset's name (anchor.png), but its bytes
	// must be the current version's.
	entries := e.getZip("/api/wallpaper/download/zip?ids=" + aid.String())
	if got := entries["anchor.png"]; got != "CURRENT" {
		t.Errorf("zip entry body = %q, want CURRENT", got)
	}
}
