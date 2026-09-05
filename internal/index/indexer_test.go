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
