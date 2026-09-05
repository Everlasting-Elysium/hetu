package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// noopPersister is a stateless Persister for orchestration tests that assert on
// the sidecar seam rather than on persistence (the real store is exercised in
// persist_test.go). It is race-safe because it holds no mutable state.
type noopPersister struct{}

func (noopPersister) PersistAITagResult(context.Context, domain.OwnerID, domain.AssetID, domain.AITagResult) error {
	return nil
}

func newTestKernel(t *testing.T) *kernel.Kernel {
	t.Helper()
	return kernel.New(kernel.Deps{
		Log:       discardLogger(),
		Store:     nil, // orchestration never touches the store
		ThumbDir:  t.TempDir(),
		JobBuffer: 8,
	})
}

func mustAsset(t *testing.T, id, path string) domain.Asset {
	t.Helper()
	assetID, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := domain.NewOwnerID("test")
	if err != nil {
		t.Fatal(err)
	}
	return domain.Asset{ID: assetID, Owner: owner, Kind: domain.KindImage, Provider: "local", StoragePath: path, Name: "cat.jpg"}
}

// TestSubscribe_TagsIndexedAsset proves the full seam: an EventAssetIndexed is
// turned into an ai_tag job that a worker runs against the sidecar with the
// asset's storage path as the ref.
func TestSubscribe_TagsIndexedAsset(t *testing.T) {
	refCh := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tag" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var ref AssetRef
		_ = json.NewDecoder(r.Body).Decode(&ref)
		writeJSON(w, http.StatusOK, TagResult{Tags: []Tag{{Name: "cat"}}})
		refCh <- ref.Ref
	}))
	t.Cleanup(srv.Close)

	k := newTestKernel(t)
	Subscribe(k, testClient(srv.URL), noopPersister{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	k.Jobs.Start(ctx, 2)

	k.Events.Publish(ctx, kernel.Event{Type: kernel.EventAssetIndexed, Data: mustAsset(t, "a1", "photos/cat.jpg")})

	select {
	case ref := <-refCh:
		if ref != "photos/cat.jpg" {
			t.Errorf("tag ref = %q, want %q", ref, "photos/cat.jpg")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the ai_tag job to reach the sidecar")
	}
}

// TestSubscribe_IgnoresNonAssetEvents proves the type guard: a malformed event
// payload never produces a job, while valid events still do.
func TestSubscribe_IgnoresNonAssetEvents(t *testing.T) {
	var calls atomic.Int32
	done := make(chan struct{}, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusOK, TagResult{})
		done <- struct{}{}
	}))
	t.Cleanup(srv.Close)

	k := newTestKernel(t)
	Subscribe(k, testClient(srv.URL), noopPersister{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	k.Jobs.Start(ctx, 2)

	k.Events.Publish(ctx, kernel.Event{Type: kernel.EventAssetIndexed, Data: "garbage"})
	k.Events.Publish(ctx, kernel.Event{Type: kernel.EventAssetIndexed, Data: mustAsset(t, "a1", "one.jpg")})
	k.Events.Publish(ctx, kernel.Event{Type: kernel.EventAssetIndexed, Data: mustAsset(t, "a2", "two.jpg")})

	for range 2 {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for valid tag calls")
		}
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("tag calls = %d, want 2 (the garbage event must be ignored)", got)
	}
}

// TestTagJob_SkipsOnNotImplemented proves a 501 from the stub sidecar is a
// graceful skip (nil error), not a job failure.
func TestTagJob_SkipsOnNotImplemented(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"status": "not_implemented"})
	}))
	t.Cleanup(srv.Close)

	o := &orchestrator{tagger: testClient(srv.URL), store: noopPersister{}, log: discardLogger()}
	if err := o.tagJob(mustAsset(t, "a1", "cat.jpg"))(context.Background()); err != nil {
		t.Fatalf("501 must be a graceful skip, got %v", err)
	}
}

// TestTagJob_ReturnsErrorOnFailure proves a genuine failure surfaces so the
// JobQueue worker records it.
func TestTagJob_ReturnsErrorOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	o := &orchestrator{tagger: testClient(srv.URL), store: noopPersister{}, log: discardLogger()}
	err := o.tagJob(mustAsset(t, "a1", "cat.jpg"))(context.Background())
	if err == nil {
		t.Fatal("expected an error for a 400 response")
	}
	var aerr *Error
	if !errors.As(err, &aerr) || aerr.Kind != KindInvalid {
		t.Errorf("expected wrapped invalid *Error, got %v", err)
	}
}
