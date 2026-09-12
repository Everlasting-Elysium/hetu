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
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/api"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/plugins/dam"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// compareResp mirrors the compare endpoint's JSON. The per-dimension detail is
// kept as RawMessage so a nil/omitted dimension is observable as a zero-length
// message (the whole point of the "omit unavailable dimensions" contract).
type compareResp struct {
	ReqID        string          `json:"req_id"`
	Dimensions   []string        `json:"dimensions"`
	OverallScore float64         `json:"overall_score"`
	Color        json.RawMessage `json:"color"`
	Tone         json.RawMessage `json:"tone"`
	Lighting     json.RawMessage `json:"lighting"`
	Action       json.RawMessage `json:"action"`
	Element      json.RawMessage `json:"element"`
	Overlays     struct {
		ReferenceURL string `json:"reference_url"`
		TargetURL    string `json:"target_url"`
	} `json:"overlays"`
	Critique struct {
		Available bool `json:"available"`
	} `json:"critique"`
}

// compareFixture builds a router with a local storage provider and no Tagger/
// Embedder (the default: synchronous tagging unavailable), returning the server,
// owner, store, and library dir for seeding library-asset sides.
func compareFixture(t *testing.T) (*httptest.Server, domain.OwnerID, kernel.Store, string) {
	t.Helper()
	srv, owner, st, lib, _ := compareFixtureWithKernel(t)
	return srv, owner, st, lib
}

// compareFixtureWithKernel is compareFixture plus the kernel, so a test can wire
// an optional service (e.g. k.VisionCritic) before issuing requests. The default
// kernel has a nil VisionCritic, matching production before an AI sidecar is set.
func compareFixtureWithKernel(t *testing.T) (*httptest.Server, domain.OwnerID, kernel.Store, string, *kernel.Kernel) {
	t.Helper()
	ctx := context.Background()
	lib := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "compare.db"))
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
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	k := kernel.New(kernel.Deps{Log: log, Store: st, CompareDir: t.TempDir(), JobBuffer: 1})
	k.Storage.Register(local.New(lib))
	p := dam.New(owner)
	if err := p.Init(ctx, k); err != nil {
		t.Fatalf("init plugin: %v", err)
	}
	srv := httptest.NewServer(api.NewRouter(k, []kernel.Plugin{p}, fstest.MapFS{}))
	t.Cleanup(srv.Close)
	return srv, owner, st, lib, k
}

// makeComparePNG encodes a solid-color w×h PNG (a real, decodable body).
func makeComparePNG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// seedCompareAsset writes a real PNG into lib and indexes it as an image asset.
func seedCompareAsset(t *testing.T, st kernel.Store, owner domain.OwnerID, lib, id, name string, c color.RGBA) {
	t.Helper()
	data := makeComparePNG(16, 16, c)
	if err := os.WriteFile(filepath.Join(lib, name), data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.UpsertAsset(context.Background(), domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: local.ProviderName,
		StoragePath: name, Name: name, Ext: "png", Size: int64(len(data)),
		Hash: "h-" + id, Width: 16, Height: 16, CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert asset: %v", err)
	}
}

// comparePart is one multipart entry: a file when filename is set, else a field.
type comparePart struct {
	field    string
	filename string
	data     []byte
	value    string
}

// postCompare posts a multipart body of the given parts to POST /compare.
func postCompare(t *testing.T, srvURL string, parts ...comparePart) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, part := range parts {
		if part.filename != "" {
			fw, err := mw.CreateFormFile(part.field, part.filename)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fw.Write(part.data); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := mw.WriteField(part.field, part.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srvURL+"/api/dam/compare", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// decodeCompare asserts a 200 and decodes the body.
func decodeCompare(t *testing.T, resp *http.Response) compareResp {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, b)
	}
	var out compareResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCompare_BothLibraryAssets(t *testing.T) {
	srv, owner, st, lib := compareFixture(t)
	seedCompareAsset(t, st, owner, lib, "ref1", "ref.png", color.RGBA{R: 200, G: 80, B: 40, A: 255})
	seedCompareAsset(t, st, owner, lib, "tgt1", "tgt.png", color.RGBA{R: 40, G: 90, B: 210, A: 255})

	got := decodeCompare(t, postCompare(t, srv.URL,
		comparePart{field: "reference_asset_id", value: "ref1"},
		comparePart{field: "target_asset_id", value: "tgt1"},
	))

	if got.ReqID == "" {
		t.Fatal("req_id empty")
	}
	if len(got.Color) == 0 || len(got.Tone) == 0 || len(got.Lighting) == 0 {
		t.Fatalf("color/tone/lighting must be present; color=%s tone=%s lighting=%s",
			got.Color, got.Tone, got.Lighting)
	}
	if len(got.Action) != 0 || len(got.Element) != 0 {
		t.Fatalf("action/element must be omitted with nil Tagger; action=%s element=%s",
			got.Action, got.Element)
	}
	if !equalStrings(got.Dimensions, []string{"color", "tone", "lighting"}) {
		t.Fatalf("dimensions = %v, want [color tone lighting]", got.Dimensions)
	}
	if got.OverallScore < 0 || got.OverallScore > 100 {
		t.Fatalf("overall_score = %v, want within [0,100]", got.OverallScore)
	}
	if got.Critique.Available {
		t.Fatal("critique.available must be false in this PR")
	}
}

func TestCompare_BothUploads_NoDBRecords(t *testing.T) {
	srv, owner, st, _ := compareFixture(t)
	ctx := context.Background()

	got := decodeCompare(t, postCompare(t, srv.URL,
		comparePart{field: "reference_file", filename: "ref.png", data: makeComparePNG(16, 16, color.RGBA{R: 220, G: 40, B: 40, A: 255})},
		comparePart{field: "target_file", filename: "tgt.png", data: makeComparePNG(16, 16, color.RGBA{R: 40, G: 40, B: 220, A: 255})},
	))
	if len(got.Color) == 0 {
		t.Fatal("color missing for upload compare")
	}

	// Directly query the store: a transient upload creates NO asset row...
	assets, err := st.ListAssets(ctx, owner, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 {
		t.Fatalf("ListAssets = %d, want 0 (uploads must never be indexed)", len(assets))
	}
	// ...and NO embedding row (SearchByEmbedding scans the embeddings table).
	matches, err := st.SearchByEmbedding(ctx, owner, []float32{1, 0, 0}, domain.AssetID{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("SearchByEmbedding = %d, want 0 (uploads must never be embedded)", len(matches))
	}
}

func TestCompare_MixedAssetAndUpload(t *testing.T) {
	srv, owner, st, lib := compareFixture(t)
	seedCompareAsset(t, st, owner, lib, "ref1", "ref.png", color.RGBA{R: 200, G: 80, B: 40, A: 255})

	got := decodeCompare(t, postCompare(t, srv.URL,
		comparePart{field: "reference_asset_id", value: "ref1"},
		comparePart{field: "target_file", filename: "tgt.png", data: makeComparePNG(16, 16, color.RGBA{R: 40, G: 40, B: 220, A: 255})},
	))
	if got.ReqID == "" || len(got.Color) == 0 {
		t.Fatalf("mixed compare failed: %+v", got)
	}
	// The library side must not create extra rows beyond the one seeded asset.
	assets, err := st.ListAssets(context.Background(), owner, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("ListAssets = %d, want 1 (only the seeded asset)", len(assets))
	}
}

func TestCompare_DimensionsSubset(t *testing.T) {
	srv, owner, st, lib := compareFixture(t)
	seedCompareAsset(t, st, owner, lib, "ref1", "ref.png", color.RGBA{R: 200, G: 80, B: 40, A: 255})
	seedCompareAsset(t, st, owner, lib, "tgt1", "tgt.png", color.RGBA{R: 40, G: 90, B: 210, A: 255})

	got := decodeCompare(t, postCompare(t, srv.URL,
		comparePart{field: "reference_asset_id", value: "ref1"},
		comparePart{field: "target_asset_id", value: "tgt1"},
		comparePart{field: "dimensions", value: "color"},
	))
	if !equalStrings(got.Dimensions, []string{"color"}) {
		t.Fatalf("dimensions = %v, want [color]", got.Dimensions)
	}
	if len(got.Tone) != 0 || len(got.Lighting) != 0 {
		t.Fatalf("tone/lighting must be omitted when only color requested")
	}
}

func TestCompare_NonImageUpload400(t *testing.T) {
	srv, _, _, _ := compareFixture(t)
	resp := postCompare(t, srv.URL,
		comparePart{field: "reference_file", filename: "notes.txt", data: []byte("plain text, definitely not an image file")},
		comparePart{field: "target_file", filename: "tgt.png", data: makeComparePNG(16, 16, color.RGBA{A: 255})},
	)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for non-image upload", resp.StatusCode)
	}
}

func TestCompare_UnknownAsset404(t *testing.T) {
	srv, owner, st, lib := compareFixture(t)
	seedCompareAsset(t, st, owner, lib, "tgt1", "tgt.png", color.RGBA{R: 40, G: 90, B: 210, A: 255})
	resp := postCompare(t, srv.URL,
		comparePart{field: "reference_asset_id", value: "ghost"},
		comparePart{field: "target_asset_id", value: "tgt1"},
	)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown asset_id", resp.StatusCode)
	}
}

func TestCompareOverlay(t *testing.T) {
	srv, _, _, _ := compareFixture(t)
	got := decodeCompare(t, postCompare(t, srv.URL,
		comparePart{field: "reference_file", filename: "ref.png", data: makeComparePNG(16, 16, color.RGBA{R: 220, G: 40, B: 40, A: 255})},
		comparePart{field: "target_file", filename: "tgt.png", data: makeComparePNG(16, 16, color.RGBA{R: 40, G: 40, B: 220, A: 255})},
	))

	// Both overlays stream back non-empty bytes.
	for _, u := range []string{got.Overlays.ReferenceURL, got.Overlays.TargetURL} {
		resp, err := http.Get(srv.URL + u)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("overlay %s status = %d, want 200", u, resp.StatusCode)
		}
		if len(body) == 0 {
			t.Fatalf("overlay %s returned empty body", u)
		}
	}

	// Unknown reqID -> 404 (uuid.Parse rejects it before any disk access).
	resp, err := http.Get(srv.URL + "/api/dam/compare/does-not-exist/overlay/reference")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown reqID status = %d, want 404", resp.StatusCode)
	}

	// Invalid side -> 400 (validated before the reqID).
	resp, err = http.Get(srv.URL + "/api/dam/compare/" + got.ReqID + "/overlay/left")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid side status = %d, want 400", resp.StatusCode)
	}
}
