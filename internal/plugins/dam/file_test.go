package dam_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// fileFixture returns a running server whose owner has one indexed asset backed
// by a real file on a local storage provider, plus the asset id and the library
// dir (so tests can mutate the backing file, e.g. delete it to simulate #45).
func fileFixture(t *testing.T, name string, data []byte) (*httptest.Server, string, string) {
	t.Helper()
	ctx := context.Background()

	lib := t.TempDir()
	if err := os.WriteFile(filepath.Join(lib, name), data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
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

	aid, err := domain.NewAssetID("vid1")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindVideo, Provider: local.ProviderName,
		StoragePath: name, Name: name, Ext: "mp4", Size: int64(len(data)),
		Hash: "h1", CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
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
	return srv, aid.String(), lib
}

func TestServeFile_Full(t *testing.T) {
	data := []byte("hetu-media-body-0123456789")
	srv, id, _ := fileFixture(t, "clip.mp4", data)

	resp, err := http.Get(srv.URL + "/api/dam/assets/" + id + "/file")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type = %q, want video/mp4", ct)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "bytes" {
		t.Errorf("Accept-Ranges = %q, want bytes", ar)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(data) {
		t.Errorf("body = %q, want %q", body, data)
	}
}

func TestServeFile_Range(t *testing.T) {
	data := []byte("hetu-media-body-0123456789")
	srv, id, _ := fileFixture(t, "clip.mp4", data)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/dam/assets/"+id+"/file", nil)
	req.Header.Set("Range", "bytes=5-9")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	if cr := resp.Header.Get("Content-Range"); cr != "bytes 5-9/26" {
		t.Errorf("Content-Range = %q, want bytes 5-9/26", cr)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(data[5:10]) {
		t.Errorf("body = %q, want %q", body, data[5:10])
	}
}

func TestServeFile_NotFound(t *testing.T) {
	srv, _, _ := fileFixture(t, "clip.mp4", []byte("x"))

	// A well-formed but unknown id resolves to 404, not a 500.
	missing, err := domain.NewAssetID("ghost")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/api/dam/assets/" + missing.String() + "/file")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServeFile_MissingOnDisk(t *testing.T) {
	// Asset row exists but the backing file was removed from storage (issue #45
	// missing state): Stat fails, so the endpoint returns 404 rather than 500.
	srv, id, lib := fileFixture(t, "clip.mp4", []byte("data"))
	if err := os.Remove(filepath.Join(lib, "clip.mp4")); err != nil {
		t.Fatalf("remove backing file: %v", err)
	}
	resp, err := http.Get(srv.URL + "/api/dam/assets/" + id + "/file")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
