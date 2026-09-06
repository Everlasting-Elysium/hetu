package dam_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/Everlasting-Elysium/hetu/internal/api"
	assetimage "github.com/Everlasting-Elysium/hetu/internal/asset/image"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/index"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/plugins/dam"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

type versionResp struct {
	ID        string `json:"id"`
	VersionNo int    `json:"version_no"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Note      string `json:"note"`
	IsCurrent bool   `json:"is_current"`
}

type assetResp struct {
	ID     string `json:"id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Thumb  string `json:"thumb"`
}

// TestVersionLifecycle drives the whole feature end-to-end against a real store,
// local provider, and image handler: scan an anchor image, upload a new version
// (lazy v1 backfill + v2 current), confirm reads reflect the current version,
// roll back, delete the old version, and prove the managed tree is never scanned.
func TestVersionLifecycle(t *testing.T) {
	srv, owner, st, k, lib := newVersionServer(t)
	ctx := context.Background()

	// Anchor: a 320x240 PNG indexed in place.
	writeTestPNG(t, filepath.Join(lib, "design.png"), 320, 240)
	if _, err := index.New(k, owner).Scan(ctx, local.ProviderName, ""); err != nil {
		t.Fatalf("scan: %v", err)
	}
	assets := listAssets(t, srv.URL)
	if len(assets) != 1 {
		t.Fatalf("after scan: %d assets, want 1", len(assets))
	}
	assetID := assets[0].ID
	if assets[0].Width != 320 || assets[0].Height != 240 {
		t.Fatalf("anchor dims = %dx%d, want 320x240", assets[0].Width, assets[0].Height)
	}

	// Upload version 2: a 100x80 PNG. It becomes current.
	up := uploadVersion(t, srv.URL+"/api/dam/assets/"+assetID+"/versions", "design.png", makePNG(100, 80), "revised")
	if up.VersionNo != 2 || !up.IsCurrent {
		t.Fatalf("upload = %+v, want version_no=2 is_current=true", up)
	}

	// The version file lives under the managed tree, not in place.
	if !hasManagedVersionFile(t, lib, assetID) {
		t.Fatal("uploaded version file not found under .hetu/versions")
	}
	// The original anchor file is untouched.
	if _, err := os.Stat(filepath.Join(lib, "design.png")); err != nil {
		t.Fatalf("anchor file disturbed: %v", err)
	}

	// List: two versions, v2 current with the note, v1 backfilled as "initial".
	versions := listVersions(t, srv.URL, assetID)
	if len(versions) != 2 {
		t.Fatalf("versions = %d, want 2", len(versions))
	}
	if versions[0].VersionNo != 2 || !versions[0].IsCurrent || versions[0].Note != "revised" {
		t.Fatalf("v2 = %+v, want no=2 current note=revised", versions[0])
	}
	if versions[1].VersionNo != 1 || versions[1].IsCurrent || versions[1].Note != "initial" {
		t.Fatalf("v1 = %+v, want no=1 not-current note=initial", versions[1])
	}

	// Read reflects the current version (v2 = 100x80), not the anchor.
	if a := getAsset(t, srv.URL, assetID); a.Width != 100 || a.Height != 80 {
		t.Fatalf("current dims = %dx%d, want 100x80 (v2)", a.Width, a.Height)
	}

	// Roll back to v1: reads reflect the anchor again (320x240).
	setCurrent(t, srv.URL, assetID, 1, http.StatusOK)
	if a := getAsset(t, srv.URL, assetID); a.Width != 320 || a.Height != 240 {
		t.Fatalf("after rollback dims = %dx%d, want 320x240 (v1)", a.Width, a.Height)
	}

	// Deleting the current version (v1) is refused.
	if code := deleteVersion(t, srv.URL, assetID, 1); code != http.StatusConflict {
		t.Fatalf("delete current status = %d, want 409", code)
	}
	// Deleting the non-current v2 succeeds and removes its managed file.
	if code := deleteVersion(t, srv.URL, assetID, 2); code != http.StatusOK {
		t.Fatalf("delete v2 status = %d, want 200", code)
	}
	if hasManagedVersionFile(t, lib, assetID) {
		t.Fatal("v2 managed file not removed after delete")
	}
	if v := listVersions(t, srv.URL, assetID); len(v) != 1 || v[0].VersionNo != 1 {
		t.Fatalf("after delete versions = %+v, want only v1", v)
	}

	// A rescan must not index the managed version tree as new assets.
	if _, err := index.New(k, owner).Scan(ctx, local.ProviderName, ""); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if a := listAssets(t, srv.URL); len(a) != 1 {
		t.Fatalf("after rescan: %d assets, want 1 (managed tree excluded)", len(a))
	}
	_ = st
}

// --- helpers ---

func newVersionServer(t *testing.T) (*httptest.Server, domain.OwnerID, *store.SQLite, *kernel.Kernel, string) {
	t.Helper()
	ctx := context.Background()
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, filepath.Join(tmp, "dam.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatalf("ensure owner: %v", err)
	}
	k := kernel.New(kernel.Deps{
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:     st,
		ThumbDir:  filepath.Join(tmp, "thumbs"),
		JobBuffer: 1,
	})
	k.Storage.Register(local.New(lib))
	k.Assets.Register(assetimage.New())
	p := dam.New(owner)
	if err := p.Init(ctx, k); err != nil {
		t.Fatalf("init plugin: %v", err)
	}
	srv := httptest.NewServer(api.NewRouter(k, []kernel.Plugin{p}, fstest.MapFS{}))
	t.Cleanup(srv.Close)
	return srv, owner, st, k, lib
}

func uploadVersion(t *testing.T, url, filename string, body []byte, note string) versionResp {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if note != "" {
		if err := mw.WriteField("note", note); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d, want 201; body=%s", resp.StatusCode, b)
	}
	var out versionResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode upload: %v", err)
	}
	return out
}

func listVersions(t *testing.T, base, assetID string) []versionResp {
	t.Helper()
	resp, err := http.Get(base + "/api/dam/assets/" + assetID + "/versions")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("list versions status = %d, want 200; body=%s", resp.StatusCode, b)
	}
	var out []versionResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode versions: %v", err)
	}
	return out
}

func listAssets(t *testing.T, base string) []assetResp {
	t.Helper()
	resp, err := http.Get(base + "/api/dam/assets")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out []assetResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode assets: %v", err)
	}
	return out
}

func getAsset(t *testing.T, base, assetID string) assetResp {
	t.Helper()
	for _, a := range listAssets(t, base) {
		if a.ID == assetID {
			return a
		}
	}
	t.Fatalf("asset %s not found", assetID)
	return assetResp{}
}

func setCurrent(t *testing.T, base, assetID string, no, wantStatus int) {
	t.Helper()
	url := base + "/api/dam/assets/" + assetID + "/versions/" + itoa(no) + "/current"
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("set current status = %d, want %d", resp.StatusCode, wantStatus)
	}
}

func deleteVersion(t *testing.T, base, assetID string, no int) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, base+"/api/dam/assets/"+assetID+"/versions/"+itoa(no), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// hasManagedVersionFile reports whether any file exists under the asset's managed
// version directory (lib/.hetu/versions/<assetID>/...).
func hasManagedVersionFile(t *testing.T, lib, assetID string) bool {
	t.Helper()
	dir := filepath.Join(lib, domain.ManagedDirName, "versions", assetID)
	var found bool
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			found = true
		}
		return nil
	})
	return found
}

func makePNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func writeTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.WriteFile(path, makePNG(w, h), 0o644); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
