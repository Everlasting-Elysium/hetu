package dam_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/api"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/plugins/dam"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

type frameDTO struct {
	FrameNo int    `json:"frame_no"`
	Name    string `json:"name"`
}

// seqFixture builds a server whose owner has a sequence asset anchored at
// frame_1.png with three real frame files on a local provider, plus a plain
// (frameless) image asset. It returns the server, owner, store, and the three
// frame bodies so a test can byte-compare the served frames.
func seqFixture(t *testing.T) (*httptest.Server, domain.OwnerID, kernel.Store, [][]byte) {
	t.Helper()
	ctx := context.Background()

	lib := t.TempDir()
	bodies := [][]byte{{0xFF, 0xD8, 0xFF, 1}, {0xFF, 0xD8, 0xFF, 2}, {0xFF, 0xD8, 0xFF, 3}}
	names := []string{"frame_1.png", "frame_2.png", "frame_3.png"}
	for i, n := range names {
		if err := os.WriteFile(filepath.Join(lib, n), bodies[i], 0o600); err != nil {
			t.Fatalf("write frame: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(lib, "plain.png"), []byte{0xFF, 0xD8, 0xFF, 9}, 0o600); err != nil {
		t.Fatalf("write plain: %v", err)
	}

	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "dam.db"))
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
	seedImage(t, st, owner, "seq", "frame_1.png")
	seedImage(t, st, owner, "plain", "plain.png")

	frames := make([]domain.AssetFrame, len(names))
	for i, n := range names {
		frames[i] = domain.AssetFrame{FrameNo: i + 1, StoragePath: n, Name: n}
	}
	if err := st.ReplaceAssetFrames(ctx, owner, local.ProviderName, "frame_1.png", frames); err != nil {
		t.Fatalf("replace frames: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	k := kernel.New(kernel.Deps{Log: log, Store: st, ThumbDir: t.TempDir(), JobBuffer: 1})
	k.Storage.Register(local.New(lib))
	p := dam.New(owner)
	if err := p.Init(ctx, k); err != nil {
		t.Fatalf("init plugin: %v", err)
	}
	srv := httptest.NewServer(api.NewRouter(k, []kernel.Plugin{p}, fstest.MapFS{}))
	t.Cleanup(srv.Close)
	return srv, owner, st, bodies
}

// seedImage upserts an image asset (provider local) so ReplaceAssetFrames can
// resolve it by natural key and requireAsset can find it.
func seedImage(t *testing.T, st kernel.Store, owner domain.OwnerID, id, path string) domain.AssetID {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(context.Background(), domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: local.ProviderName,
		StoragePath: path, Name: id, Ext: "png", Size: 1, Hash: id,
		CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert image: %v", err)
	}
	return aid
}

func TestListAssetFrames(t *testing.T) {
	srv, _, _, _ := seqFixture(t)
	frames := getFrames(t, srv.URL+"/api/dam/assets/seq/frames", http.StatusOK)
	if len(frames) != 3 {
		t.Fatalf("frames = %d, want 3", len(frames))
	}
	for i, fr := range frames {
		wantNo := i + 1
		wantName := "frame_" + strconv.Itoa(wantNo) + ".png"
		if fr.FrameNo != wantNo || fr.Name != wantName {
			t.Errorf("frame[%d] = {no:%d name:%q}, want {no:%d name:%q}", i, fr.FrameNo, fr.Name, wantNo, wantName)
		}
	}
}

func TestServeFrame(t *testing.T) {
	srv, _, _, bodies := seqFixture(t)

	// Each frame streams its own original bytes with an image content type.
	for n := 1; n <= 3; n++ {
		resp, err := http.Get(srv.URL + "/api/dam/assets/seq/frames/" + strconv.Itoa(n))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			t.Fatalf("frame %d status = %d, want 200", n, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
			t.Errorf("frame %d content-type = %q, want image/png", n, ct)
		}
		got, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if !bytes.Equal(got, bodies[n-1]) {
			t.Errorf("frame %d body = %v, want %v", n, got, bodies[n-1])
		}
	}

	// A valid-but-absent frame is 404; non-numeric or < 1 is 400.
	assertStatus(t, srv.URL+"/api/dam/assets/seq/frames/9", http.StatusNotFound)
	assertStatus(t, srv.URL+"/api/dam/assets/seq/frames/abc", http.StatusBadRequest)
	assertStatus(t, srv.URL+"/api/dam/assets/seq/frames/0", http.StatusBadRequest)
}

// TestAssetFramesEmpty asserts a plain (non-sequence) image returns 200 with [].
func TestAssetFramesEmpty(t *testing.T) {
	srv, _, _, _ := seqFixture(t)
	frames := getFrames(t, srv.URL+"/api/dam/assets/plain/frames", http.StatusOK)
	if len(frames) != 0 {
		t.Errorf("frames = %d, want 0", len(frames))
	}
}

// TestAssetFramesNotFound covers a non-existent asset on both endpoints.
func TestAssetFramesNotFound(t *testing.T) {
	srv, _, _, _ := seqFixture(t)
	assertStatus(t, srv.URL+"/api/dam/assets/ghost/frames", http.StatusNotFound)
	assertStatus(t, srv.URL+"/api/dam/assets/ghost/frames/1", http.StatusNotFound)
}

// TestAssetFramesCrossOwnerIsolation asserts one owner cannot read another
// owner's frames or frame bytes (IDOR): both return 404.
func TestAssetFramesCrossOwnerIsolation(t *testing.T) {
	srv, _, st, _ := seqFixture(t)
	ctx := context.Background()

	other, err := domain.NewOwnerID("intruder-target")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, other); err != nil {
		t.Fatalf("ensure other owner: %v", err)
	}
	seedImage(t, st, other, "secret", "secret_1.png")
	if err := st.ReplaceAssetFrames(ctx, other, local.ProviderName, "secret_1.png", []domain.AssetFrame{
		{FrameNo: 1, StoragePath: "secret_1.png", Name: "secret_1.png"},
		{FrameNo: 2, StoragePath: "secret_2.png", Name: "secret_2.png"},
	}); err != nil {
		t.Fatalf("replace frames: %v", err)
	}

	assertStatus(t, srv.URL+"/api/dam/assets/secret/frames", http.StatusNotFound)
	assertStatus(t, srv.URL+"/api/dam/assets/secret/frames/1", http.StatusNotFound)
}

func getFrames(t *testing.T, url string, wantStatus int) []frameDTO {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, wantStatus)
	}
	var out []frameDTO
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}
