package store_test

import (
	"errors"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func TestGetAssetByPath(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	id := seedAsset(t, ctx, st, owner, "a1", "sub/pic.png")

	got, err := st.GetAssetByPath(ctx, owner, "local", "sub/pic.png")
	if err != nil {
		t.Fatalf("get by path: %v", err)
	}
	if got.ID != id {
		t.Errorf("id = %s, want %s", got.ID, id)
	}

	if _, err := st.GetAssetByPath(ctx, owner, "local", "nope.png"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing path err = %v, want ErrNotFound", err)
	}
}

func TestUpsertAnnotation_SearchableCaption(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	id := seedAsset(t, ctx, st, owner, "a1", "pic.png")

	// A manual caption becomes the FTS description, so it is searchable.
	if err := st.UpsertAnnotation(ctx, owner, domain.Annotation{
		AssetID: id, Layer: domain.LayerManual, Key: domain.KeyCaption, Value: `"a calm harbour"`,
	}); err != nil {
		t.Fatalf("upsert annotation: %v", err)
	}
	hits, err := st.SearchAssets(ctx, owner, "harbour", 10, 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != id {
		t.Fatalf("search hits = %v, want the captioned asset", hits)
	}
}

func TestUpdateJob_StatusAndPayload(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	jobID, err := domain.NewJobID("j1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnqueueJob(ctx, domain.Job{
		ID: jobID, Owner: owner, Type: "import_eagle", Status: domain.JobPending, Payload: "{}",
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := st.UpdateJob(ctx, owner, jobID, domain.JobDone, `{"imported":3}`); err != nil {
		t.Fatalf("update job: %v", err)
	}
	jobs, err := st.ListJobs(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Status != domain.JobDone || jobs[0].Payload != `{"imported":3}` {
		t.Fatalf("job = %+v, want done + payload", jobs[0])
	}
}
