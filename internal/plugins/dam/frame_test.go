package dam_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// seedAsset upserts a minimal asset of the given kind so the frame endpoint's
// param and kind gates can be exercised without a storage provider — those
// checks return before any file access.
func seedAsset(t *testing.T, st kernel.Store, owner domain.OwnerID, rawID, ext string, kind domain.AssetKind) string {
	t.Helper()
	aid, err := domain.NewAssetID(rawID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(context.Background(), domain.Asset{
		ID: aid, Owner: owner, Kind: kind, Provider: "local",
		StoragePath: rawID + "." + ext, Name: rawID, Ext: ext, Size: 1,
		Hash: rawID, CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert %s: %v", rawID, err)
	}
	return aid.String()
}

func TestExtractFrame_NotVideo(t *testing.T) {
	srv, owner, st := newTestServer(t)
	id := seedAsset(t, st, owner, "img1", "png", domain.KindImage)

	resp, err := http.Get(srv.URL + "/api/dam/assets/" + id + "/frame?ms=0")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("not-a-video status = %d, want 400", resp.StatusCode)
	}
}

func TestExtractFrame_MissingMs(t *testing.T) {
	srv, owner, st := newTestServer(t)
	id := seedAsset(t, st, owner, "vid1", "mp4", domain.KindVideo)

	resp, err := http.Get(srv.URL + "/api/dam/assets/" + id + "/frame")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing ms status = %d, want 400", resp.StatusCode)
	}
}

func TestExtractFrame_InvalidMs(t *testing.T) {
	srv, owner, st := newTestServer(t)
	id := seedAsset(t, st, owner, "vid1", "mp4", domain.KindVideo)

	resp, err := http.Get(srv.URL + "/api/dam/assets/" + id + "/frame?ms=abc")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid ms status = %d, want 400", resp.StatusCode)
	}
}

// TestExtractFrame_WithFFmpeg exercises the real decode path when ffmpeg is on
// PATH: it generates a short clip, then asserts the endpoint returns a JPEG with
// the immutable cache header.
func TestExtractFrame_WithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	srv, id, _ := fileFixture(t, "clip.mp4", makeTestVideo(t))

	resp, err := http.Get(srv.URL + "/api/dam/assets/" + id + "/frame?ms=500")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=86400, immutable" {
		t.Errorf("Cache-Control = %q, want public, max-age=86400, immutable", cc)
	}
	body, _ := io.ReadAll(resp.Body)
	// JPEG SOI marker (0xFF 0xD8) proves ffmpeg produced a real image.
	if len(body) < 2 || body[0] != 0xFF || body[1] != 0xD8 {
		t.Fatalf("body is not a JPEG (len=%d)", len(body))
	}
}

// makeTestVideo renders a 1s lavfi testsrc clip with ffmpeg and returns its bytes
// so a fileFixture-backed asset points at a genuinely decodable video.
func makeTestVideo(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "src.mp4")
	cmd := exec.Command("ffmpeg", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=15",
		"-pix_fmt", "yuv420p", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate test video: %v\n%s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test video: %v", err)
	}
	return data
}
