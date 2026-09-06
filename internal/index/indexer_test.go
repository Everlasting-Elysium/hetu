package index_test

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	assetimage "github.com/Everlasting-Elysium/hetu/internal/asset/image"
	"github.com/Everlasting-Elysium/hetu/internal/asset/model3d"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/index"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// TestIndexer_Scan drives the full chain against a real store, a real local
// provider, and the real image handler (no mocks): a PNG is indexed and a
// thumbnail is written; a non-image file is skipped.
func TestIndexer_Scan(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(lib, "a.png"), 320, 240)
	if err := os.WriteFile(filepath.Join(lib, "skip.txt"), []byte("x"), 0o644); err != nil {
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
	res, err := index.New(k, owner).Scan(ctx, local.ProviderName, "")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if res.Indexed != 1 || res.Skipped != 1 {
		t.Fatalf("res = %+v, want {Indexed:1 Skipped:1}", res)
	}

	assets, err := st.ListAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("len = %d, want 1", len(assets))
	}
	a := assets[0]
	if a.Width != 320 || a.Height != 240 {
		t.Fatalf("dims = %dx%d, want 320x240", a.Width, a.Height)
	}
	if a.ThumbPath == "" {
		t.Fatal("thumb path empty")
	}
	if _, err := os.Stat(a.ThumbPath); err != nil {
		t.Fatalf("thumbnail not written: %v", err)
	}
}

// TestIndexer_PublishesAssetIndexed proves the indexer announces every indexed
// asset on EventAssetIndexed (the seam the AI tagging pipeline subscribes to).
// The bus is synchronous, so the handler runs inside Scan on this goroutine.
func TestIndexer_PublishesAssetIndexed(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(lib, "a.png"), 16, 16)
	if err := os.WriteFile(filepath.Join(lib, "skip.txt"), []byte("x"), 0o644); err != nil {
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

	var indexed []domain.Asset
	k.Events.Subscribe(kernel.EventAssetIndexed, func(_ context.Context, e kernel.Event) {
		a, ok := e.Data.(domain.Asset)
		if !ok {
			t.Errorf("event data = %T, want domain.Asset", e.Data)
			return
		}
		indexed = append(indexed, a)
	})

	owner, err := domain.NewOwnerID("t")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := index.New(k, owner).Scan(ctx, local.ProviderName, ""); err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Exactly one event: the PNG is indexed, the .txt is skipped (no event).
	if len(indexed) != 1 {
		t.Fatalf("EventAssetIndexed count = %d, want 1", len(indexed))
	}
	if indexed[0].Kind != domain.KindImage || indexed[0].StoragePath == "" {
		t.Errorf("published asset = %+v, want an image with a storage path", indexed[0])
	}
}

// TestIndexer_ScanModel drives the chain with the 3D model handler and no
// Blender sidecar (empty addr): the .obj is indexed as a model with no
// dimensions and no thumbnail (graceful degradation), the .txt is skipped.
func TestIndexer_ScanModel(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	const obj = "v 0 0 0\nv 1 0 0\nv 0 1 0\nf 1 2 3\n"
	if err := os.WriteFile(filepath.Join(lib, "tri.obj"), []byte(obj), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "skip.txt"), []byte("x"), 0o644); err != nil {
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
	k.Assets.Register(model3d.New(""))

	owner, err := domain.NewOwnerID("t")
	if err != nil {
		t.Fatal(err)
	}
	res, err := index.New(k, owner).Scan(ctx, local.ProviderName, "")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if res.Indexed != 1 || res.Skipped != 1 {
		t.Fatalf("res = %+v, want {Indexed:1 Skipped:1}", res)
	}

	assets, err := st.ListAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("len = %d, want 1", len(assets))
	}
	a := assets[0]
	if a.Kind != domain.KindModel {
		t.Fatalf("kind = %q, want %q", a.Kind, domain.KindModel)
	}
	if a.ThumbPath != "" {
		t.Fatalf("thumb path = %q, want empty (no blender)", a.ThumbPath)
	}
	if a.Width != 0 {
		t.Fatalf("width = %d, want 0", a.Width)
	}
}

// TestIndexer_DetectMissing proves that a rescan flags an asset as missing once
// its backing file is deleted: the first scan indexes the PNG, the file is
// removed, and the second scan marks the record missing (missing_at set).
func TestIndexer_DetectMissing(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(lib, "a.png"), 32, 32)

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
	ix := index.New(k, owner)

	if _, err := ix.Scan(ctx, local.ProviderName, ""); err != nil {
		t.Fatalf("first scan: %v", err)
	}

	// Delete the physical file, then rescan: the asset must be marked missing.
	if err := os.Remove(filepath.Join(lib, "a.png")); err != nil {
		t.Fatal(err)
	}
	res, err := ix.Scan(ctx, local.ProviderName, "")
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if res.Missing != 1 {
		t.Fatalf("res.Missing = %d, want 1", res.Missing)
	}

	missing, err := st.ListMissingAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 {
		t.Fatalf("missing count = %d, want 1", len(missing))
	}
	if missing[0].MissingAt == nil {
		t.Fatal("missing_at not set on missing asset")
	}
}

// TestIndexer_AutoReconnect proves that moving a file to a new path preserves
// its asset identity: the moved file reconnects (by content hash) to its
// now-missing record in a single rescan instead of creating a duplicate, and
// missing_at is cleared.
func TestIndexer_AutoReconnect(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(lib, "a.png"), 32, 32)

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
	ix := index.New(k, owner)

	if _, err := ix.Scan(ctx, local.ProviderName, ""); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	before, err := st.ListAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 {
		t.Fatalf("after first scan len = %d, want 1", len(before))
	}
	origID := before[0].ID

	// Move the file to a new path within the same library.
	if err := os.Rename(filepath.Join(lib, "a.png"), filepath.Join(lib, "b.png")); err != nil {
		t.Fatal(err)
	}

	res, err := ix.Scan(ctx, local.ProviderName, "")
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if res.Reconnected != 1 {
		t.Fatalf("res.Reconnected = %d, want 1", res.Reconnected)
	}

	after, err := st.ListAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("after reconnect len = %d, want 1 (no duplicate)", len(after))
	}
	if after[0].ID != origID {
		t.Fatalf("asset ID = %s, want preserved %s", after[0].ID, origID)
	}
	if after[0].StoragePath != "b.png" {
		t.Fatalf("path = %q, want %q", after[0].StoragePath, "b.png")
	}
	if after[0].MissingAt != nil {
		t.Fatal("missing_at not cleared after reconnect")
	}
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
