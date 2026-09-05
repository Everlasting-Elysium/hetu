package dam_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/api"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/plugins/dam"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// fakeEmbedder is a kernel.Embedder returning a canned vector (or error).
type fakeEmbedder struct {
	vec []float32
	err error
}

func (f fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return f.vec, f.err
}

// seedSemanticRouter builds a router over a store with two embedded assets and
// the given embedder. When embedder is nil, semantic search is disabled.
func seedSemanticRouter(t *testing.T, embedder kernel.Embedder) http.Handler {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	owner, err := domain.NewOwnerID("t")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	seed := func(id, path string, vec []float32) {
		aid, err := domain.NewAssetID(id)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Truncate(time.Second)
		if err := st.UpsertAsset(ctx, domain.Asset{
			ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
			StoragePath: path, Name: path, Ext: "png", Size: 1, Hash: id,
			CreatedAt: now, IndexedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := st.IndexEmbedding(ctx, aid, vec, "clip-test"); err != nil {
			t.Fatal(err)
		}
	}
	seed("cat", "cat.png", []float32{1, 0, 0})
	seed("dog", "dog.png", []float32{0, 1, 0})

	k := kernel.New(kernel.Deps{
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:     st,
		JobBuffer: 1,
	})
	k.Embedder = embedder
	p := dam.New(owner)
	if err := p.Init(ctx, k); err != nil {
		t.Fatal(err)
	}
	return api.NewRouter(k, []kernel.Plugin{p}, fstest.MapFS{})
}

func TestSearchBySemantic_HTTP(t *testing.T) {
	// Query vector aligned with cat: cat ranks first.
	router := seedSemanticRouter(t, fakeEmbedder{vec: []float32{0.9, 0.1, 0}})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dam/search?semantic=a+cat", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got []struct {
		Name       string  `json:"name"`
		Similarity float64 `json:"similarity"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].Name != "cat.png" {
		t.Fatalf("results = %+v, want cat.png first", got)
	}
	// Similarity is rounded to 4 decimals.
	if got[0].Similarity <= got[1].Similarity {
		t.Fatalf("not ranked: %+v", got)
	}
}

func TestSearchBySemantic_NoEmbedder503(t *testing.T) {
	router := seedSemanticRouter(t, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dam/search?semantic=cat", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestSearchBySemantic_EmptyQuery400(t *testing.T) {
	router := seedSemanticRouter(t, fakeEmbedder{vec: []float32{1, 0, 0}})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dam/search?semantic=+++", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for blank query", rec.Code)
	}
}

func TestSearchBySemantic_EmbedError502(t *testing.T) {
	router := seedSemanticRouter(t, fakeEmbedder{err: io.ErrUnexpectedEOF})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dam/search?semantic=cat", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

func TestSearchBySimilar_HTTP(t *testing.T) {
	router := seedSemanticRouter(t, nil) // similar needs no embedder
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dam/search?similar=cat", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Self (cat) is excluded; only dog remains.
	if len(got) != 1 || got[0].Name != "dog.png" {
		t.Fatalf("results = %+v, want only dog.png (self excluded)", got)
	}
}

func TestSearchBySimilar_UnknownID404(t *testing.T) {
	router := seedSemanticRouter(t, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dam/search?similar=nonexistent", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
