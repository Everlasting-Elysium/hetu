package index_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	assetimage "github.com/Everlasting-Elysium/hetu/internal/asset/image"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/index"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// seqScanBed builds an Indexer over a real store, a real local provider, and the
// real image handler, returning the indexer, store, owner, and library dir so a
// test can seed/mutate numbered PNGs and rescan.
func seqScanBed(t *testing.T) (*index.Indexer, kernel.Store, domain.OwnerID, string) {
	t.Helper()
	ctx := context.Background()
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, filepath.Join(tmp, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	k := kernel.New(kernel.Deps{
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:     st,
		ThumbDir:  filepath.Join(tmp, "thumbs"),
		JobBuffer: 1,
	})
	k.Storage.Register(local.New(lib))
	k.Assets.Register(assetimage.New())
	owner, err := domain.NewOwnerID("t")
	if err != nil {
		t.Fatal(err)
	}
	return index.New(k, owner), st, owner, lib
}

func assetByPath(t *testing.T, st kernel.Store, owner domain.OwnerID, path string) domain.Asset {
	t.Helper()
	assets, err := st.ListAssets(context.Background(), owner, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range assets {
		if a.StoragePath == path {
			return a
		}
	}
	t.Fatalf("no asset with path %q (have %d assets)", path, len(assets))
	return domain.Asset{}
}

func frameCount(t *testing.T, st kernel.Store, owner domain.OwnerID, id domain.AssetID) int {
	t.Helper()
	frames, err := st.ListAssetFrames(context.Background(), owner, id)
	if err != nil {
		t.Fatal(err)
	}
	return len(frames)
}

// TestIndexer_ScanSequence_GroupsFrames proves a run of consecutively-numbered
// images is indexed as ONE anchor asset with an N-row frame index (not N
// assets), while a standalone image stays its own frameless asset and a
// non-image is skipped.
func TestIndexer_ScanSequence_GroupsFrames(t *testing.T) {
	ctx := context.Background()
	ix, st, owner, lib := seqScanBed(t)
	for i := 1; i <= 4; i++ {
		writePNG(t, filepath.Join(lib, fmt.Sprintf("explosion_%04d.png", i)), 32, 32)
	}
	writePNG(t, filepath.Join(lib, "hero.png"), 32, 32) // standalone, no numbered siblings
	if err := os.WriteFile(filepath.Join(lib, "skip.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ix.Scan(ctx, local.ProviderName, "")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	// One sequence anchor + one standalone image = 2 indexed; the .txt skipped.
	if res.Indexed != 2 || res.Skipped != 1 {
		t.Fatalf("res = %+v, want {Indexed:2 Skipped:1}", res)
	}
	assets, err := st.ListAssets(ctx, owner, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 {
		t.Fatalf("assets = %d, want 2 (4 frames collapse into 1 anchor + hero.png)", len(assets))
	}

	anchor := assetByPath(t, st, owner, "explosion_0001.png")
	frames, err := st.ListAssetFrames(ctx, owner, anchor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(frames))
	}
	for i, fr := range frames {
		wantNo := i + 1
		wantPath := fmt.Sprintf("explosion_%04d.png", wantNo)
		if fr.FrameNo != wantNo || fr.StoragePath != wantPath || fr.Name != wantPath {
			t.Errorf("frame[%d] = {no:%d path:%q name:%q}, want {no:%d path:%q}",
				i, fr.FrameNo, fr.StoragePath, fr.Name, wantNo, wantPath)
		}
	}

	// The standalone image is not a sequence.
	hero := assetByPath(t, st, owner, "hero.png")
	if got := frameCount(t, st, owner, hero.ID); got != 0 {
		t.Errorf("hero frames = %d, want 0", got)
	}
}

// TestIndexer_ScanSequence_RebuildsOnRescan proves the frame index is rebuilt
// wholesale on every scan: it grows when a frame is added, shrinks when frames
// are removed, and is cleared entirely once the run drops below two files (the
// former anchor is a plain image again, with no stale frame rows).
func TestIndexer_ScanSequence_RebuildsOnRescan(t *testing.T) {
	ctx := context.Background()
	ix, st, owner, lib := seqScanBed(t)
	for i := 1; i <= 4; i++ {
		writePNG(t, filepath.Join(lib, fmt.Sprintf("seq_%03d.png", i)), 16, 16)
	}
	if _, err := ix.Scan(ctx, local.ProviderName, ""); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	anchor := assetByPath(t, st, owner, "seq_001.png")
	if got := frameCount(t, st, owner, anchor.ID); got != 4 {
		t.Fatalf("initial frames = %d, want 4", got)
	}

	// Grow: add a 5th frame -> rescan rebuilds to 5.
	writePNG(t, filepath.Join(lib, "seq_005.png"), 16, 16)
	if _, err := ix.Scan(ctx, local.ProviderName, ""); err != nil {
		t.Fatalf("grow scan: %v", err)
	}
	if got := frameCount(t, st, owner, anchor.ID); got != 5 {
		t.Fatalf("after grow frames = %d, want 5", got)
	}

	// Shrink: remove two frames -> rescan rebuilds to 3 (no stale rows).
	for _, n := range []string{"seq_004.png", "seq_005.png"} {
		if err := os.Remove(filepath.Join(lib, n)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ix.Scan(ctx, local.ProviderName, ""); err != nil {
		t.Fatalf("shrink scan: %v", err)
	}
	if got := frameCount(t, st, owner, anchor.ID); got != 3 {
		t.Fatalf("after shrink frames = %d, want 3", got)
	}

	// Shrink to a single file: the run drops below two, so it is no longer a
	// sequence and the former anchor's frame rows are cleared.
	for _, n := range []string{"seq_002.png", "seq_003.png"} {
		if err := os.Remove(filepath.Join(lib, n)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ix.Scan(ctx, local.ProviderName, ""); err != nil {
		t.Fatalf("single scan: %v", err)
	}
	if got := frameCount(t, st, owner, anchor.ID); got != 0 {
		t.Fatalf("after shrink-to-single frames = %d, want 0 (cleared)", got)
	}
}
