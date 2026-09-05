package index_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	assetimage "github.com/Everlasting-Elysium/hetu/internal/asset/image"
	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/index"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// TestIndexer_Palette drives the full chain and then searches by color: a solid
// PNG is scanned, its palette extracted and indexed, and a query for the fill
// color finds it while a distant color does not.
func TestIndexer_Palette(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	// writePNG (indexer_test.go) fills every pixel with RGBA{100,150,200}.
	writePNG(t, filepath.Join(lib, "solid.png"), 64, 64)

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
	if _, err := index.New(k, owner).Scan(ctx, local.ProviderName, ""); err != nil {
		t.Fatalf("scan: %v", err)
	}

	fill := color.RGB{R: 100, G: 150, B: 200}
	matches, err := st.SearchByColor(ctx, owner, fill.Lab(), 5, 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("search by fill color = %d matches, want 1", len(matches))
	}
	if matches[0].Asset.StoragePath != "solid.png" {
		t.Fatalf("matched %q, want solid.png", matches[0].Asset.StoragePath)
	}
	if matches[0].Distance > 1 {
		t.Fatalf("dominant swatch distance = %v, want ~0", matches[0].Distance)
	}

	far := color.RGB{R: 255, G: 128, B: 0} // orange, far from the blue-grey fill
	if m, err := st.SearchByColor(ctx, owner, far.Lab(), 5, 50); err != nil || len(m) != 0 {
		t.Fatalf("distant color tol=5 = %d matches (err=%v), want 0", len(m), err)
	}
}
