package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// recordingPersister captures IndexEmbedding calls for assertions.
type recordingPersister struct {
	mu    sync.Mutex
	calls []recordedEmbed
}

type recordedEmbed struct {
	assetID string
	vector  []float32
	model   string
}

func (p *recordingPersister) IndexEmbedding(_ context.Context, assetID domain.AssetID, embedding []float32, model string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, recordedEmbed{assetID: assetID.String(), vector: embedding, model: model})
	return nil
}

func (p *recordingPersister) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// TestSubscribeEmbedding_EmbedsIndexedAsset proves the full seam: an
// EventAssetIndexed becomes an ai_embed job that embeds the asset and persists
// the returned vector.
func TestSubscribeEmbedding_EmbedsIndexedAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, EmbedResult{Vector: []float32{0.1, 0.2, 0.3}, Dim: 3, Model: "clip-stub"})
	}))
	t.Cleanup(srv.Close)

	persister := &recordingPersister{}
	k := newTestKernel(t)
	SubscribeEmbedding(k, testClient(srv.URL), persister)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	k.Jobs.Start(ctx, 2)

	k.Events.Publish(ctx, kernel.Event{Type: kernel.EventAssetIndexed, Data: mustAsset(t, "a1", "photos/cat.jpg")})

	deadline := time.After(2 * time.Second)
	for persister.count() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for ai_embed to persist")
		case <-time.After(10 * time.Millisecond):
		}
	}
	persister.mu.Lock()
	defer persister.mu.Unlock()
	got := persister.calls[0]
	if got.assetID != "a1" || got.model != "clip-stub" || len(got.vector) != 3 {
		t.Fatalf("persisted = %+v, want a1/clip-stub/dim3", got)
	}
}

// TestEmbedJob_SkipsOnNotImplemented proves a 501 is a graceful skip (nil error,
// no persist).
func TestEmbedJob_SkipsOnNotImplemented(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"status": "not_implemented"})
	}))
	t.Cleanup(srv.Close)

	persister := &recordingPersister{}
	o := &embedOrchestrator{embedder: testClient(srv.URL), store: persister, log: discardLogger()}
	if err := o.embedJob(mustAsset(t, "a1", "cat.jpg"))(context.Background()); err != nil {
		t.Fatalf("501 must be a graceful skip, got %v", err)
	}
	if persister.count() != 0 {
		t.Fatalf("persisted %d, want 0 on skip", persister.count())
	}
}

// TestEmbedJob_SkipsOnEmptyVector proves an empty vector from the sidecar is a
// graceful skip, never persisting a zero-length embedding.
func TestEmbedJob_SkipsOnEmptyVector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, EmbedResult{Vector: []float32{}, Dim: 0, Model: "clip-stub"})
	}))
	t.Cleanup(srv.Close)

	persister := &recordingPersister{}
	o := &embedOrchestrator{embedder: testClient(srv.URL), store: persister, log: discardLogger()}
	if err := o.embedJob(mustAsset(t, "a1", "cat.jpg"))(context.Background()); err != nil {
		t.Fatalf("empty vector must be a graceful skip, got %v", err)
	}
	if persister.count() != 0 {
		t.Fatalf("persisted %d, want 0 on empty vector", persister.count())
	}
}

// TestEmbedJob_ReturnsErrorOnFailure proves a genuine failure surfaces so the
// worker records it.
func TestEmbedJob_ReturnsErrorOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	persister := &recordingPersister{}
	o := &embedOrchestrator{embedder: testClient(srv.URL), store: persister, log: discardLogger()}
	if err := o.embedJob(mustAsset(t, "a1", "cat.jpg"))(context.Background()); err == nil {
		t.Fatal("expected an error for a 400 response")
	}
}
