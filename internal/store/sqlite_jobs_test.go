package store_test

import (
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func mkJob(t *testing.T, owner domain.OwnerID, id, typ string, status domain.JobStatus) domain.Job {
	t.Helper()
	jid, err := domain.NewJobID(id)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Job{
		ID: jid, Owner: owner, Type: typ, Status: status,
		Payload:   `{"asset_id":"a1"}`,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
}

func TestSQLite_JobEnqueueAndList(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	// ListJobs orders newest first; make created_at deterministic.
	older := mkJob(t, owner, "j1", "thumbnail", domain.JobPending)
	older.CreatedAt = older.CreatedAt.Add(-time.Hour)
	newer := mkJob(t, owner, "j2", "ai_tag", domain.JobPending)
	if err := st.EnqueueJob(ctx, older); err != nil {
		t.Fatalf("enqueue older: %v", err)
	}
	if err := st.EnqueueJob(ctx, newer); err != nil {
		t.Fatalf("enqueue newer: %v", err)
	}

	jobs, err := st.ListJobs(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	if jobs[0].ID != newer.ID || jobs[1].ID != older.ID {
		t.Fatalf("order = %s,%s, want j2,j1 (newest first)", jobs[0].ID, jobs[1].ID)
	}
	if jobs[0].Type != "ai_tag" || jobs[0].Status != domain.JobPending ||
		jobs[0].Payload != `{"asset_id":"a1"}` {
		t.Fatalf("job fields mismatch: %+v", jobs[0])
	}
}

func TestSQLite_JobUpdateStatus(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	j := mkJob(t, owner, "j1", "thumbnail", domain.JobPending)
	if err := st.EnqueueJob(ctx, j); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if err := st.UpdateJobStatus(ctx, owner, j.ID, domain.JobRunning); err != nil {
		t.Fatalf("update running: %v", err)
	}
	jobs, err := st.ListJobs(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Status != domain.JobRunning {
		t.Fatalf("status = %v, want running", jobs[0].Status)
	}

	if err := st.UpdateJobStatus(ctx, owner, j.ID, domain.JobDone); err != nil {
		t.Fatalf("update done: %v", err)
	}
	jobs, err = st.ListJobs(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].Status != domain.JobDone {
		t.Fatalf("status = %v, want done", jobs[0].Status)
	}
}

func TestSQLite_JobListEmpty(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	jobs, err := st.ListJobs(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs = %d, want 0", len(jobs))
	}
}
