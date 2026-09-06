package importers_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	assetimage "github.com/Everlasting-Elysium/hetu/internal/asset/image"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/importers"
	"github.com/Everlasting-Elysium/hetu/internal/importers/eagle"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/storage/fs"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

const eagleLib = "eagle/testdata/sample.library"

type harness struct {
	k      *kernel.Kernel
	owner  domain.OwnerID
	st     *store.SQLite
	dbPath string
	root   string // local provider root (import copy/move target)
}

func newHarness(t *testing.T) harness {
	t.Helper()
	ctx := context.Background()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "t.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	k := kernel.New(kernel.Deps{
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:     st,
		ThumbDir:  filepath.Join(tmp, "thumbs"),
		JobBuffer: 1,
	})
	k.Storage.Register(local.New(tmp))
	k.Storage.Register(fs.New())
	k.Assets.Register(assetimage.New())
	owner, err := domain.NewOwnerID("t")
	if err != nil {
		t.Fatal(err)
	}
	return harness{k: k, owner: owner, st: st, dbPath: dbPath, root: tmp}
}

func TestImportSource_Eagle_Index(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	src, err := eagle.Open(eagleLib)
	if err != nil {
		t.Fatalf("open eagle: %v", err)
	}
	res, err := importers.New(h.k, h.owner).ImportSource(ctx, src,
		importers.Options{Mode: importers.ModeIndex}, domain.JobID{})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Imported != 3 || res.Failed != 0 {
		t.Fatalf("res = %+v, want Imported:3 Failed:0", res)
	}

	assets, err := h.st.ListAssets(ctx, h.owner, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 3 {
		t.Fatalf("assets = %d, want 3", len(assets))
	}
	byName := map[string]domain.Asset{}
	for _, a := range assets {
		byName[a.Name] = a
	}
	sunset := byName["sunset.png"]

	// Index mode: fs provider, absolute path, created_at from btime (2023).
	if sunset.Provider != fs.ProviderName || !filepath.IsAbs(sunset.StoragePath) {
		t.Errorf("provider/path = %q/%q, want fs + absolute", sunset.Provider, sunset.StoragePath)
	}
	if sunset.Rating != 5 {
		t.Errorf("rating = %d, want 5", sunset.Rating)
	}
	if sunset.CreatedAt.Year() != 2023 {
		t.Errorf("createdAt = %v, want btime year 2023", sunset.CreatedAt)
	}

	// Hierarchical folders created and the primary assigned.
	folders, _ := h.st.ListFolders(ctx, h.owner)
	paths := map[string]string{}
	for _, f := range folders {
		paths[f.Path] = f.ID.String()
	}
	for _, want := range []string{"Photos", "Illustrations", "Illustrations/Characters"} {
		if _, ok := paths[want]; !ok {
			t.Errorf("missing folder %q (have %v)", want, paths)
		}
	}
	if sunset.FolderID != paths["Photos"] {
		t.Errorf("sunset folder = %q, want Photos", sunset.FolderID)
	}

	// Tags attached; note→caption and url→source.url annotations written.
	tags, _ := h.st.ListAssetTags(ctx, sunset.ID)
	names := map[string]bool{}
	for _, tg := range tags {
		names[tg.Name] = true
	}
	if !names["nature"] || !names["nature/sky"] {
		t.Errorf("tags = %v, want nature + nature/sky", names)
	}
	if got := annotation(t, h.dbPath, sunset.ID.String(), "extracted", "caption"); got != `"A calm sunset over the sea"` {
		t.Errorf("caption = %s", got)
	}
	if got := annotation(t, h.dbPath, sunset.ID.String(), "extracted", "source.url"); got != `"https://example.com/sunset"` {
		t.Errorf("source.url = %s", got)
	}
}

func TestImportSource_Eagle_Idempotent(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	importOnce := func() {
		src, err := eagle.Open(eagleLib)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if _, err := importers.New(h.k, h.owner).ImportSource(ctx, src,
			importers.Options{Mode: importers.ModeIndex}, domain.JobID{}); err != nil {
			t.Fatalf("import: %v", err)
		}
	}
	importOnce()
	importOnce() // fresh service each run (single-run caches)

	assets, _ := h.st.ListAssets(ctx, h.owner, 100, 0)
	if len(assets) != 3 {
		t.Fatalf("assets after re-import = %d, want 3 (idempotent)", len(assets))
	}
	folders, _ := h.st.ListFolders(ctx, h.owner)
	if len(folders) != 3 {
		t.Errorf("folders = %d, want 3 (no dupes)", len(folders))
	}
}

func TestImportSource_Eagle_ConflictSkip(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	src, _ := eagle.Open(eagleLib)
	if _, err := importers.New(h.k, h.owner).ImportSource(ctx, src,
		importers.Options{Mode: importers.ModeIndex}, domain.JobID{}); err != nil {
		t.Fatalf("import: %v", err)
	}
	src2, _ := eagle.Open(eagleLib)
	res, err := importers.New(h.k, h.owner).ImportSource(ctx, src2,
		importers.Options{Mode: importers.ModeIndex, Conflict: importers.ConflictSkip}, domain.JobID{})
	if err != nil {
		t.Fatalf("import skip: %v", err)
	}
	if res.Skipped != 3 || res.Imported != 0 {
		t.Fatalf("res = %+v, want Skipped:3 Imported:0", res)
	}
}

func TestImportPath_Copy(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	absSrc, err := filepath.Abs(filepath.Join(eagleLib, "images", "MEAGLE0000000000000001.info", "sunset.png"))
	if err != nil {
		t.Fatal(err)
	}
	asset, skipped, err := importers.New(h.k, h.owner).ImportPath(ctx, absSrc,
		importers.Options{Mode: importers.ModeCopy, DestSubdir: "imported"})
	if err != nil {
		t.Fatalf("copy import: %v", err)
	}
	if skipped {
		t.Fatal("unexpected skip")
	}
	if asset.Provider != local.ProviderName {
		t.Errorf("provider = %q, want local", asset.Provider)
	}
	// The copy lives under the library root; the source is untouched.
	if _, err := os.Stat(filepath.Join(h.root, asset.StoragePath)); err != nil {
		t.Errorf("copied file missing: %v", err)
	}
	if _, err := os.Stat(absSrc); err != nil {
		t.Errorf("copy must not remove source: %v", err)
	}
}

func TestImportSource_MoveRejectedForMigration(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	src, _ := eagle.Open(eagleLib)
	_, err := importers.New(h.k, h.owner).ImportSource(ctx, src,
		importers.Options{Mode: importers.ModeMove}, domain.JobID{})
	if err == nil {
		t.Fatal("move mode must be rejected for a read-only migration source")
	}
}

// annotation reads one annotation value from the test DB (white-box: the store
// exposes no annotation getter).
func annotation(t *testing.T, dbPath, assetID, layer, key string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer func() { _ = db.Close() }()
	var val string
	if err := db.QueryRow(`SELECT value FROM annotations WHERE asset_id=? AND layer=? AND "key"=?`,
		assetID, layer, key).Scan(&val); err != nil {
		t.Fatalf("query annotation %s/%s: %v", layer, key, err)
	}
	return val
}
