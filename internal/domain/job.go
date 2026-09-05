package domain

import (
	"fmt"
	"time"
)

// JobID identifies a background job.
type JobID struct{ raw string }

// NewJobID parses s into a JobID.
func NewJobID(s string) (JobID, error) {
	if s == "" {
		return JobID{}, fmt.Errorf("job id: %w", ErrEmptyID)
	}
	return JobID{raw: s}, nil
}

// String returns the raw job id.
func (id JobID) String() string { return id.raw }

// JobStatus is the lifecycle state of a background job, stored as a string.
type JobStatus string

const (
	JobPending JobStatus = "pending"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobFailed  JobStatus = "failed"
)

// Job is a persisted background task (thumbnail generation, AI tagging, 3D
// render, ...). Execution/consumption is out of scope for the store (owned by
// kernel.JobQueue and issues #8/#9); this type is the persistence record.
type Job struct {
	ID        JobID
	Owner     OwnerID
	Type      string // e.g. "thumbnail", "ai_tag", "3d_render"
	Status    JobStatus
	Payload   string // JSON-serialized job parameters
	CreatedAt time.Time
}
