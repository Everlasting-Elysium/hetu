package index

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// fakePaged is an AssetHandler + PageExtractor whose Prepare/RenderPage calls are
// counted, proving a document is prepared (converted) exactly once regardless of
// its page count.
type fakePaged struct {
	reportPages  int
	prepareCalls int
	renderCalls  int
	onRender     func()
}

func (f *fakePaged) Match(string) bool      { return true }
func (f *fakePaged) Kind() domain.AssetKind { return domain.KindDocument }
func (f *fakePaged) Extract(context.Context, io.ReadSeeker) (domain.Meta, error) {
	return domain.Meta{Kind: domain.KindDocument}, nil
}
func (f *fakePaged) Thumbnail(context.Context, io.ReadSeeker, io.Writer) error {
	return domain.ErrNoThumbnail
}
func (f *fakePaged) Prepare(context.Context, io.ReadSeeker) (kernel.PagedDocument, error) {
	f.prepareCalls++
	return &fakePrepared{parent: f}, nil
}

type fakePrepared struct{ parent *fakePaged }

func (d *fakePrepared) PageCount(context.Context) (int, error) { return d.parent.reportPages, nil }
func (d *fakePrepared) RenderPage(_ context.Context, _ int, w io.Writer) error {
	d.parent.renderCalls++
	if d.parent.onRender != nil {
		d.parent.onRender()
	}
	_, err := w.Write([]byte{0xFF, 0xD8, 0xFF})
	return err
}
func (d *fakePrepared) Close() error { return nil }

// pagedTestbed builds an Indexer over a real store + local provider with one
// document asset already upserted (so ReplaceDocumentPages resolves it by natural
// key), returning the indexer, that asset's id, and the provider.
func pagedTestbed(t *testing.T) (*Indexer, domain.AssetID, kernel.StorageProvider) {
	t.Helper()
	ctx := context.Background()
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "doc.bin"), []byte("x"), 0o644); err != nil {
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
	prov := local.New(lib)
	k.Storage.Register(prov)
	owner, err := domain.NewOwnerID("t")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	aid, err := domain.NewAssetID("doc-asset")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindDocument, Provider: prov.Name(),
		StoragePath: "doc.bin", Name: "doc", Ext: "bin", Size: 1, Hash: "h",
		CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return New(k, owner), aid, prov
}

// TestIndexPages_PreparesOnceForManyPages proves a multi-page document is prepared
// (converted) exactly once, with every page rendered from that single
// preparation — not re-prepared per page (the N+2-conversions bug).
func TestIndexPages_PreparesOnceForManyPages(t *testing.T) {
	ix, aid, prov := pagedTestbed(t)
	h := &fakePaged{reportPages: 3}
	ix.indexPages(context.Background(), prov, "doc.bin", aid.String(), h)
	if h.prepareCalls != 1 {
		t.Fatalf("prepareCalls = %d, want 1 (one conversion for all pages)", h.prepareCalls)
	}
	if h.renderCalls != 3 {
		t.Fatalf("renderCalls = %d, want 3", h.renderCalls)
	}
	pages, err := ix.k.Store.ListDocumentPages(context.Background(), ix.owner, aid)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 {
		t.Fatalf("stored pages = %d, want 3", len(pages))
	}
}

// TestIndexPages_CapsRunawayPageCount proves a header reporting a huge page count
// is truncated to maxDocumentPages instead of spinning nearly forever.
func TestIndexPages_CapsRunawayPageCount(t *testing.T) {
	ix, aid, prov := pagedTestbed(t)
	h := &fakePaged{reportPages: 2_000_000_000}
	ix.indexPages(context.Background(), prov, "doc.bin", aid.String(), h)
	if h.renderCalls != maxDocumentPages {
		t.Fatalf("renderCalls = %d, want capped at %d", h.renderCalls, maxDocumentPages)
	}
}

// TestIndexPages_StopsOnContextCancel proves the render loop exits promptly when
// the scan context is cancelled, keeping already-rendered pages.
func TestIndexPages_StopsOnContextCancel(t *testing.T) {
	ix, aid, prov := pagedTestbed(t)
	ctx, cancel := context.WithCancel(context.Background())
	h := &fakePaged{reportPages: 50}
	h.onRender = cancel // cancel during the first page's render
	ix.indexPages(ctx, prov, "doc.bin", aid.String(), h)
	if h.renderCalls != 1 {
		t.Fatalf("renderCalls = %d, want 1 (loop exits right after cancel)", h.renderCalls)
	}
}
