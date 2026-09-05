package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// EmbedJobName is the kernel.Job name for CLIP embedding jobs.
const EmbedJobName = "ai_embed"

// Embedder is the slice of [*Client] the embedding orchestrator depends on.
type Embedder interface {
	Embed(ctx context.Context, ref AssetRef) (EmbedResult, error)
}

// EmbedPersister writes CLIP embedding vectors to the store.
type EmbedPersister interface {
	IndexEmbedding(ctx context.Context, assetID domain.AssetID, embedding []float32, model string) error
}

// SubscribeEmbedding wires CLIP embedding onto the kernel: for every
// EventAssetIndexed it enqueues an [EmbedJobName] job that embeds the asset via
// the sidecar and persists the vector. A sidecar 501 is a graceful skip.
func SubscribeEmbedding(k *kernel.Kernel, embedder Embedder, store EmbedPersister) {
	o := &embedOrchestrator{jobs: k.Jobs, embedder: embedder, store: store, log: k.Log}
	k.Events.Subscribe(kernel.EventAssetIndexed, o.onIndexed)
}

type embedOrchestrator struct {
	jobs     *kernel.JobQueue
	embedder Embedder
	store    EmbedPersister
	log      *slog.Logger
}

func (o *embedOrchestrator) onIndexed(ctx context.Context, e kernel.Event) {
	asset, ok := e.Data.(domain.Asset)
	if !ok {
		return
	}
	job := kernel.Job{Name: EmbedJobName, Run: o.embedJob(asset)}
	go func() {
		ectx, cancel := context.WithTimeout(ctx, enqueueTimeout)
		defer cancel()
		if err := o.jobs.Enqueue(ectx, job); err != nil {
			o.log.DebugContext(ectx, "ai_embed enqueue dropped",
				slog.String("asset", asset.ID.String()), slog.Any("err", err))
		}
	}()
}

func (o *embedOrchestrator) embedJob(asset domain.Asset) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := embedAndPersist(ctx, o.embedder, o.store, asset, o.log)
		return err
	}
}

// embedAndPersist embeds one asset via the sidecar and persists the vector. A
// 501 from the stub sidecar or an empty vector is a graceful skip (skipped=true,
// err=nil), not a failure; every other embedder or store error is returned
// wrapped. This is the shared core of both the event-driven job
// ([embedOrchestrator.embedJob]) and the batch [EmbedAll] pass.
func embedAndPersist(ctx context.Context, embedder Embedder, store EmbedPersister, asset domain.Asset, log *slog.Logger) (skipped bool, err error) {
	assetID := asset.ID.String()
	res, err := embedder.Embed(ctx, AssetRef{Ref: asset.StoragePath})
	if err != nil {
		if errors.Is(err, ErrNotImplemented) {
			log.InfoContext(ctx, "ai_embed skipped: sidecar capability not implemented",
				slog.String("asset", assetID))
			return true, nil
		}
		return false, fmt.Errorf("ai_embed %s: %w", assetID, err)
	}
	if len(res.Vector) == 0 {
		log.WarnContext(ctx, "ai_embed skipped: sidecar returned empty vector",
			slog.String("asset", assetID), slog.String("model", res.Model))
		return true, nil
	}
	if err := store.IndexEmbedding(ctx, asset.ID, res.Vector, res.Model); err != nil {
		return false, fmt.Errorf("ai_embed persist %s: %w", assetID, err)
	}
	log.InfoContext(ctx, "ai_embed persisted",
		slog.String("asset", assetID),
		slog.Int("dim", res.Dim),
		slog.String("model", res.Model))
	return false, nil
}
