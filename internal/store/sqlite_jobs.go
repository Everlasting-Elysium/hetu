package store

import (
	"context"
	"fmt"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// EnqueueJob inserts a new background job.
func (s *SQLite) EnqueueJob(ctx context.Context, j domain.Job) error {
	if err := s.q.EnqueueJob(ctx, db.EnqueueJobParams{
		ID:        j.ID.String(),
		OwnerID:   j.Owner.String(),
		Type:      j.Type,
		Status:    string(j.Status),
		Payload:   j.Payload,
		CreatedAt: j.CreatedAt.Unix(),
	}); err != nil {
		return fmt.Errorf("enqueue job %s: %w", j.ID, err)
	}
	return nil
}

// UpdateJobStatus transitions the owner's job to status.
func (s *SQLite) UpdateJobStatus(ctx context.Context, owner domain.OwnerID, id domain.JobID, status domain.JobStatus) error {
	if err := s.q.UpdateJobStatus(ctx, db.UpdateJobStatusParams{
		Status:  string(status),
		ID:      id.String(),
		OwnerID: owner.String(),
	}); err != nil {
		return fmt.Errorf("update job status %s: %w", id, err)
	}
	return nil
}

// UpdateJob transitions the owner's job to status and replaces its payload in
// one statement, so a long-running job (e.g. a migration import) can persist
// progress counts (JSON) in the payload as it advances without a schema change.
func (s *SQLite) UpdateJob(ctx context.Context, owner domain.OwnerID, id domain.JobID, status domain.JobStatus, payload string) error {
	if err := s.q.UpdateJobProgress(ctx, db.UpdateJobProgressParams{
		Status:  string(status),
		Payload: payload,
		ID:      id.String(),
		OwnerID: owner.String(),
	}); err != nil {
		return fmt.Errorf("update job %s: %w", id, err)
	}
	return nil
}

// ListJobs returns the owner's jobs, newest first.
func (s *SQLite) ListJobs(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Job, error) {
	rows, err := s.q.ListJobs(ctx, db.ListJobsParams{
		OwnerID: owner.String(),
		Limit:   int64(limit),
		Offset:  int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	jobs := make([]domain.Job, 0, len(rows))
	for _, r := range rows {
		j, err := rowToJob(r)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func rowToJob(r db.Job) (domain.Job, error) {
	id, err := domain.NewJobID(r.ID)
	if err != nil {
		return domain.Job{}, fmt.Errorf("row job id: %w", err)
	}
	owner, err := domain.NewOwnerID(r.OwnerID)
	if err != nil {
		return domain.Job{}, fmt.Errorf("row job owner: %w", err)
	}
	return domain.Job{
		ID:        id,
		Owner:     owner,
		Type:      r.Type,
		Status:    domain.JobStatus(r.Status),
		Payload:   r.Payload,
		CreatedAt: time.Unix(r.CreatedAt, 0).UTC(),
	}, nil
}
