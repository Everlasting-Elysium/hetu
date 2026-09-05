package kernel

import (
	"context"
	"log/slog"
)

// Job is a unit of background work (e.g. indexing a directory).
type Job struct {
	Name string
	Run  func(ctx context.Context) error
}

// JobQueue is a bounded in-process work queue drained by worker goroutines.
type JobQueue struct {
	log *slog.Logger
	ch  chan Job
}

// NewJobQueue returns a queue with the given buffer size.
func NewJobQueue(log *slog.Logger, buffer int) *JobQueue {
	return &JobQueue{log: log, ch: make(chan Job, buffer)}
}

// Enqueue submits a job. It blocks when the buffer is full or ctx is done.
func (q *JobQueue) Enqueue(ctx context.Context, j Job) error {
	select {
	case q.ch <- j:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Start launches n worker goroutines that run until ctx is cancelled.
func (q *JobQueue) Start(ctx context.Context, workers int) {
	for range workers {
		go q.worker(ctx)
	}
}

func (q *JobQueue) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-q.ch:
			if err := j.Run(ctx); err != nil {
				q.log.ErrorContext(ctx, "job failed",
					slog.String("job", j.Name), slog.Any("err", err))
			}
		}
	}
}
