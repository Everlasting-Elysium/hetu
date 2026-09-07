package dam_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

// modelEnv is a DAM server backed by a local storage provider, an optional GLB
// converter, and a GLB cache dir — enough to exercise GET .../model.
type modelEnv struct {
	srv      *httptest.Server
	owner    domain.OwnerID
	store    kernel.Store
	libDir   string
	cacheDir string
}

// fakeConverter is an in-memory kernel.ModelConverter that exercises the DAM
// plugin's GLB cache, singleflight dedup, and output validation without a real
// converter backend. It counts calls and can simulate a slow conversion (delay)
// or a corrupt one (emptyOut writes nothing, so validateGLB rejects it).
type fakeConverter struct {
	glb      []byte
	calls    atomic.Int32
	delay    time.Duration
	wantExt  string
	emptyOut bool
}

func (c *fakeConverter) ConvertToGLB(ctx context.Context, ext string, _ io.ReadSeeker, w io.Writer) error {
	c.calls.Add(1)
	if c.wantExt != "" && ext != c.wantExt {
		return fmt.Errorf("unexpected ext %q, want %q", ext, c.wantExt)
	}
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if c.emptyOut {
		return nil // 200-equivalent empty body → validateGLB fails downstream
	}
	_, err := w.Write(c.glb)
	return err
}

// newModelServer builds a DAM test server whose 3D converter is conv (nil means
// conversion is unavailable, so non-web-friendly formats return 503).
func newModelServer(t *testing.T, conv kernel.ModelConverter) modelEnv {
	t.Helper()
	ctx := context.Background()
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

	libDir, cacheDir := t.TempDir(), t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	k := kernel.New(kernel.Deps{
		Log: log, Store: st, ThumbDir: t.TempDir(),
		ModelCacheDir: cacheDir, ModelConverter: conv, JobBuffer: 1,
	})
	k.Storage.Register(local.New(libDir))
	p := dam.New(owner)
	if err := p.Init(ctx, k); err != nil {
		t.Fatalf("init plugin: %v", err)
	}
	srv := httptest.NewServer(api.NewRouter(k, []kernel.Plugin{p}, fstest.MapFS{}))
	t.Cleanup(srv.Close)
	return modelEnv{srv: srv, owner: owner, store: st, libDir: libDir, cacheDir: cacheDir}
}

// upsertModel writes content to <libDir>/<name> and indexes it as a model asset.
func (e modelEnv) upsertModel(t *testing.T, id, name, ext, hash string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.libDir, name), content, 0o644); err != nil {
		t.Fatalf("write model file: %v", err)
	}
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := e.store.UpsertAsset(context.Background(), domain.Asset{
		ID: aid, Owner: e.owner, Kind: domain.KindModel, Provider: "local",
		StoragePath: name, Name: name, Ext: ext, Size: int64(len(content)),
		Hash: hash, CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert model: %v", err)
	}
}

func getModel(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, body
}

// glbBytes returns fake GLB content that opens with the real glTF magic so it
// passes the server's post-conversion validation (see validateGLB).
func glbBytes(tag string) []byte {
	return append([]byte("glTF\x02\x00\x00\x00"), tag...)
}

func TestServeModel_WebFriendlyGLBStreamsDirect(t *testing.T) {
	e := newModelServer(t, nil)
	glb := []byte("glTF\x02\x00\x00\x00direct-glb-bytes")
	e.upsertModel(t, "m-glb", "cube.glb", "glb", "h-glb", glb)

	resp, body := getModel(t, e.srv.URL+"/api/dam/assets/m-glb/model")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "model/gltf-binary" {
		t.Errorf("content-type = %q, want model/gltf-binary", ct)
	}
	if string(body) != string(glb) {
		t.Errorf("body = %q, want the original GLB bytes", body)
	}
	// A web-friendly model must not be cached as a converted copy.
	if entries, _ := os.ReadDir(e.cacheDir); len(entries) != 0 {
		t.Errorf("cache dir has %d entries, want 0 for direct GLB", len(entries))
	}
}

func TestServeModel_ConvertsAndCaches(t *testing.T) {
	glb := glbBytes("converted-payload")
	conv := &fakeConverter{glb: glb, wantExt: "stl"}
	e := newModelServer(t, conv)
	e.upsertModel(t, "m-stl", "part.stl", "stl", "h-stl", []byte("solid binary stl bytes"))

	// First request converts and caches the GLB.
	resp, body := getModel(t, e.srv.URL+"/api/dam/assets/m-stl/model")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first status = %d, want 200", resp.StatusCode)
	}
	if string(body) != string(glb) {
		t.Errorf("body = %q, want converted GLB", body)
	}
	if _, err := os.Stat(filepath.Join(e.cacheDir, "h-stl.glb")); err != nil {
		t.Errorf("expected cached GLB at h-stl.glb: %v", err)
	}

	// Second request is served from cache — the converter is not called again.
	resp2, body2 := getModel(t, e.srv.URL+"/api/dam/assets/m-stl/model")
	if resp2.StatusCode != http.StatusOK || string(body2) != string(glb) {
		t.Errorf("second request status=%d body=%q, want 200 + cached GLB", resp2.StatusCode, body2)
	}
	if got := conv.calls.Load(); got != 1 {
		t.Errorf("converter called %d times, want 1 (cache hit on 2nd)", got)
	}
}

func TestServeModel_ConvertibleWithoutConverter503(t *testing.T) {
	e := newModelServer(t, nil) // no converter configured
	e.upsertModel(t, "m-obj", "mesh.obj", "obj", "h-obj", []byte("o mesh"))

	resp, _ := getModel(t, e.srv.URL+"/api/dam/assets/m-obj/model")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 without a converter", resp.StatusCode)
	}
}

func TestServeModel_UnsupportedExt404(t *testing.T) {
	e := newModelServer(t, nil)
	// An opaque/native format that is not a web-viewable model: 404 so the UI
	// falls back to the preview image.
	e.upsertModel(t, "m-ztl", "sculpt.ztl", "ztl", "h-ztl", []byte("ZBRUSH"))

	resp, _ := getModel(t, e.srv.URL+"/api/dam/assets/m-ztl/model")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unsupported format", resp.StatusCode)
	}
}

func TestServeModel_MissingAsset404(t *testing.T) {
	e := newModelServer(t, nil)
	resp, _ := getModel(t, e.srv.URL+"/api/dam/assets/does-not-exist/model")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for missing asset", resp.StatusCode)
	}
}

func TestServeModel_GltfStreamsDirect(t *testing.T) {
	e := newModelServer(t, nil)
	gltf := []byte(`{"asset":{"version":"2.0"}}`)
	e.upsertModel(t, "m-gltf", "scene.gltf", "gltf", "h-gltf", gltf)

	resp, body := getModel(t, e.srv.URL+"/api/dam/assets/m-gltf/model")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "model/gltf+json" {
		t.Errorf("content-type = %q, want model/gltf+json", ct)
	}
	if string(body) != string(gltf) {
		t.Errorf("body = %q, want the original gltf bytes", body)
	}
}

// A converter that returns success with an empty/corrupt body must not poison
// the cache: the request fails and no GLB is committed (validateGLB).
func TestServeModel_EmptyConversionNotCached(t *testing.T) {
	e := newModelServer(t, &fakeConverter{emptyOut: true})
	e.upsertModel(t, "m-bad", "bad.obj", "obj", "h-bad", []byte("o mesh"))

	resp, _ := getModel(t, e.srv.URL+"/api/dam/assets/m-bad/model")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for empty conversion", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(e.cacheDir, "h-bad.glb")); !os.IsNotExist(err) {
		t.Errorf("empty conversion left a cache file, want none")
	}
}

// Concurrent requests for the same un-cached model collapse into a single
// conversion via singleflight (H1).
func TestServeModel_ConcurrentConvertsOnce(t *testing.T) {
	glb := glbBytes("concurrent")
	conv := &fakeConverter{glb: glb, delay: 150 * time.Millisecond}
	e := newModelServer(t, conv)
	e.upsertModel(t, "m-cc", "many.obj", "obj", "h-cc", []byte("o mesh"))

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(e.srv.URL + "/api/dam/assets/m-cc/model")
			if err != nil {
				errs <- err
				return
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errs <- fmt.Errorf("status %d", resp.StatusCode)
				return
			}
			if string(body) != string(glb) {
				errs <- fmt.Errorf("body mismatch len=%d", len(body))
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := conv.calls.Load(); got != 1 {
		t.Errorf("converter called %d times, want 1 (singleflight dedup)", got)
	}
}
