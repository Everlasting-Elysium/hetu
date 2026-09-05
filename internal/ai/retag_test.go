package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func countAssetTags(t *testing.T, ctx context.Context, dbPath, assetID string) int {
	t.Helper()
	raw := mustRawDB(t, dbPath)
	defer func() { _ = raw.Close() }()
	var n int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM asset_tags WHERE asset_id=?`, assetID).Scan(&n); err != nil {
		t.Fatalf("count asset tags: %v", err)
	}
	return n
}

// TestRetag_tagsWholeLibraryIdempotently proves the batch pass tags every live
// asset via the sidecar and is safe to re-run (no duplicate links).
func TestRetag_tagsWholeLibraryIdempotently(t *testing.T) {
	ctx, st, owner, _, dbPath := aiTestStore(t) // seeds photo-1
	seedTestAsset(t, ctx, st, owner, "photo-2", "photos/dog.jpg")

	client := testClient(fakeTagSidecar(t).URL)
	tagged, skipped, err := Retag(ctx, client, st, owner, discardLogger())
	if err != nil {
		t.Fatalf("Retag: %v", err)
	}
	if tagged != 2 || skipped != 0 {
		t.Errorf("Retag = (tagged=%d, skipped=%d), want (2, 0)", tagged, skipped)
	}
	for _, id := range []string{"photo-1", "photo-2"} {
		if src, ok := queryTagSource(t, ctx, dbPath, id, "animal"); !ok || src != "ai" {
			t.Errorf("%s animal tag source = %q (ok=%v), want ai", id, src, ok)
		}
	}

	// Re-run: idempotent — same tag count, no error.
	if _, _, err := Retag(ctx, client, st, owner, discardLogger()); err != nil {
		t.Fatalf("Retag re-run: %v", err)
	}
	if n := countAssetTags(t, ctx, dbPath, "photo-1"); n != 2 {
		t.Errorf("photo-1 tag count after re-run = %d, want 2 (no duplicates)", n)
	}
}

// TestRetag_skipsWhenSidecarUnimplemented proves the Phase-1 stub (501 on /tag)
// makes Retag a graceful skip, not a failure — the pipeline is wired and ready
// before the real models (#11) land.
func TestRetag_skipsWhenSidecarUnimplemented(t *testing.T) {
	ctx, st, owner, _, _ := aiTestStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"status": "not_implemented"})
	}))
	t.Cleanup(srv.Close)

	tagged, skipped, err := Retag(ctx, testClient(srv.URL), st, owner, discardLogger())
	if err != nil {
		t.Fatalf("Retag with stub sidecar: %v", err)
	}
	if tagged != 0 || skipped != 1 {
		t.Errorf("Retag = (tagged=%d, skipped=%d), want (0, 1)", tagged, skipped)
	}
}
