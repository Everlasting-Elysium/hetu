package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// JobName is the kernel.Job name of the tagging job enqueued after indexing.
const JobName = "ai_tag"

// enqueueTimeout bounds how long the event handler waits to hand a job to the
// queue before dropping it, so a saturated queue (e.g. a worker-less scan) can
// never stall the indexer that published the event.
const enqueueTimeout = 2 * time.Second

// Tagger is the slice of [*Client] the orchestrator depends on. Keeping it small
// makes the dependency explicit and lets tests substitute a fake sidecar.
type Tagger interface {
	Tag(ctx context.Context, ref AssetRef) (TagResult, error)
}

// Subscribe wires the AI tagging pipeline onto the kernel: for every
// kernel.EventAssetIndexed it enqueues a [JobName] job on k.Jobs that calls
// tagger against the indexed asset. Enqueue is best-effort and detached so
// publishing never blocks the indexer; the job runs on a JobQueue worker
// (started by the server). A sidecar 501 (the Phase 1 stub) is a graceful skip,
// not a job failure. Persisting results to the ai annotation layer is #9.
func Subscribe(k *kernel.Kernel, tagger Tagger) {
	o := &orchestrator{jobs: k.Jobs, tagger: tagger, log: k.Log}
	k.Events.Subscribe(kernel.EventAssetIndexed, o.onIndexed)
}

type orchestrator struct {
	jobs   *kernel.JobQueue
	tagger Tagger
	log    *slog.Logger
}

// onIndexed enqueues a tagging job for the indexed asset. The enqueue is
// detached from the publisher's goroutine and bounded by [enqueueTimeout] so a
// full queue drops the job instead of blocking the scan.
func (o *orchestrator) onIndexed(ctx context.Context, e kernel.Event) {
	asset, ok := e.Data.(domain.Asset)
	if !ok {
		return
	}
	job := kernel.Job{Name: JobName, Run: o.tagJob(asset)}
	go func() {
		ectx, cancel := context.WithTimeout(ctx, enqueueTimeout)
		defer cancel()
		if err := o.jobs.Enqueue(ectx, job); err != nil {
			o.log.DebugContext(ectx, "ai_tag enqueue dropped",
				slog.String("asset", asset.ID.String()), slog.Any("err", err))
		}
	}()
}

// tagJob builds the closure that tags one asset via the sidecar.
func (o *orchestrator) tagJob(asset domain.Asset) func(context.Context) error {
	ref := AssetRef{Ref: asset.StoragePath}
	assetID := asset.ID.String()
	return func(ctx context.Context) error {
		res, err := o.tagger.Tag(ctx, ref)
		if err != nil {
			if errors.Is(err, ErrNotImplemented) {
				o.log.InfoContext(ctx, "ai_tag skipped: sidecar capability not implemented",
					slog.String("asset", assetID))
				return nil
			}
			return fmt.Errorf("ai_tag %s: %w", assetID, err)
		}
		o.log.InfoContext(ctx, "ai_tag completed",
			slog.String("asset", assetID), slog.Int("tags", len(res.Tags)))
		return nil
	}
}
