package importers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// progressEvery throttles job-payload writes so a large library does not issue
// one UPDATE per item; progress is also flushed once at the end.
const progressEvery = 20

// Result summarizes a migration import run.
type Result struct {
	Total    int `json:"total"`
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
}

// Progress is the JSON payload persisted to the job row as an import advances.
type Progress struct {
	Kind    string `json:"kind"`
	Mode    string `json:"mode"`
	Result         // flattened Total/Imported/Skipped/Failed
	Current string `json:"current,omitempty"`
}

// ImportSource migrates every item from src into hetu per opt. A non-empty
// jobID persists progress to that job row as the run advances (empty = no job,
// e.g. the synchronous CLI). Move mode is rejected: a migration source stays
// read-only. Per-item failures are counted and logged, never fatal.
func (s *Service) ImportSource(ctx context.Context, src Source, opt Options, jobID domain.JobID) (Result, error) {
	if opt.mode() == ModeMove {
		return Result{}, fmt.Errorf("import %s: move mode not allowed (source is read-only)", src.Kind())
	}
	var res Result
	err := src.Each(ctx, func(item ImportItem) error {
		res.Total++
		_, skipped, impErr := s.ImportItem(ctx, item, opt)
		switch {
		case impErr != nil:
			res.Failed++
			s.k.Log.WarnContext(ctx, "import item failed",
				slog.String("source", string(src.Kind())),
				slog.String("name", item.Name), slog.Any("err", impErr))
		case skipped:
			res.Skipped++
		default:
			res.Imported++
		}
		if res.Total%progressEvery == 0 {
			s.writeProgress(ctx, jobID, domain.JobRunning, src, opt, res, item.Name)
		}
		return nil
	})
	status := domain.JobDone
	if err != nil {
		status = domain.JobFailed
	}
	s.writeProgress(ctx, jobID, status, src, opt, res, "")
	return res, err
}

// StartMigration enqueues an import job for src and schedules it on the kernel
// JobQueue, returning the job id immediately so a large library imports in the
// background. Progress is readable via the job row (ListJobs). src must own its
// resources for the async run (open/close within Each).
func (s *Service) StartMigration(ctx context.Context, src Source, opt Options) (domain.JobID, error) {
	if opt.mode() == ModeMove {
		return domain.JobID{}, fmt.Errorf("import %s: move mode not allowed (source is read-only)", src.Kind())
	}
	raw := uuid.Must(uuid.NewV7()).String()
	jobID, err := domain.NewJobID(raw)
	if err != nil {
		return domain.JobID{}, err
	}
	initial, _ := json.Marshal(Progress{Kind: string(src.Kind()), Mode: string(opt.mode())})
	if err := s.k.Store.EnqueueJob(ctx, domain.Job{
		ID: jobID, Owner: s.owner, Type: "import_" + string(src.Kind()),
		Status: domain.JobPending, Payload: string(initial), CreatedAt: time.Now().UTC(),
	}); err != nil {
		return domain.JobID{}, err
	}
	if err := s.k.Jobs.Enqueue(ctx, kernel.Job{
		Name: "import_" + string(src.Kind()),
		Run: func(runCtx context.Context) error {
			_, runErr := s.ImportSource(runCtx, src, opt, jobID)
			return runErr
		},
	}); err != nil {
		// Don't leave the row stuck "pending" if scheduling failed.
		_ = s.k.Store.UpdateJobStatus(ctx, s.owner, jobID, domain.JobFailed)
		return domain.JobID{}, err
	}
	return jobID, nil
}

// writeProgress persists the run's counts to the job payload. It is a no-op when
// jobID is empty (synchronous callers) and logs but never fails on a write error.
func (s *Service) writeProgress(ctx context.Context, jobID domain.JobID, status domain.JobStatus, src Source, opt Options, res Result, current string) {
	if jobID.String() == "" {
		return
	}
	payload, err := json.Marshal(Progress{
		Kind: string(src.Kind()), Mode: string(opt.mode()), Result: res, Current: current,
	})
	if err != nil {
		return
	}
	if err := s.k.Store.UpdateJob(ctx, s.owner, jobID, status, string(payload)); err != nil {
		s.k.Log.WarnContext(ctx, "update import job progress", slog.Any("err", err))
	}
}
