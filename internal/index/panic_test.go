package index_test

import (
	"context"
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

// panicHandler is an AssetHandler that panics in the named method, standing in
// for a third-party decoder choking on a malformed file.
type panicHandler struct{ where string }

func (panicHandler) Match(ext string) bool  { return ext == "boom" }
func (panicHandler) Kind() domain.AssetKind { return domain.KindOther }
func (h panicHandler) Extract(context.Context, io.ReadSeeker) (domain.Meta, error) {
	if h.where == "extract" {
		panic("boom in Extract")
	}
	return domain.Meta{Kind: domain.KindOther}, nil
}
func (h panicHandler) Thumbnail(context.Context, io.ReadSeeker, io.Writer) error {
	if h.where == "thumbnail" {
		panic("boom in Thumbnail")
	}
	return domain.ErrNoThumbnail
}

// TestIndexer_RecoversHandlerPanic proves a handler panicking on one file does
// not crash the whole scan: the panicking file is skipped and a sibling image is
// still indexed.
func TestIndexer_RecoversHandlerPanic(t *testing.T) {
	for _, where := range []string{"extract", "thumbnail"} {
		t.Run(where, func(t *testing.T) {
			ctx := context.Background()
			tmp := t.TempDir()
			lib := filepath.Join(tmp, "lib")
			if err := os.MkdirAll(lib, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(lib, "a.boom"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			writePNG(t, filepath.Join(lib, "b.png"), 16, 16)

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
			k.Assets.Register(panicHandler{where: where})
			k.Assets.Register(assetimage.New())

			owner, err := domain.NewOwnerID("t")
			if err != nil {
				t.Fatal(err)
			}
			// Must not panic: the boom file is skipped, the PNG survives.
			res, err := index.New(k, owner).Scan(ctx, local.ProviderName, "")
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if res.Indexed != 1 {
				t.Fatalf("res.Indexed = %d, want 1 (png survives the panic)", res.Indexed)
			}
			assets, err := st.ListAssets(ctx, owner, 10, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(assets) != 1 || assets[0].StoragePath != "b.png" {
				t.Fatalf("assets = %+v, want only b.png", assets)
			}
		})
	}
}
