package wallpaper

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/api"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// testEnv is a wired wallpaper plugin behind an httptest server, backed by a
// real temp-dir SQLite store and a real local storage provider (no mocks — the
// download/zip paths need genuine file bytes).
type testEnv struct {
	t        *testing.T
	ctx      context.Context
	srv      *httptest.Server
	plugin   *Plugin
	st       *store.SQLite
	owner    domain.OwnerID
	libDir   string
	thumbDir string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "wall.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatalf("ensure owner: %v", err)
	}

	libDir, thumbDir := t.TempDir(), t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	k := kernel.New(kernel.Deps{Log: log, Store: st, ThumbDir: thumbDir, JobBuffer: 1})
	k.Storage.Register(local.New(libDir))
	p := New(owner)
	if err := p.Init(ctx, k); err != nil {
		t.Fatalf("init plugin: %v", err)
	}
	srv := httptest.NewServer(api.NewRouter(k, []kernel.Plugin{p}, fstest.MapFS{}))
	t.Cleanup(srv.Close)
	return &testEnv{t: t, ctx: ctx, srv: srv, plugin: p, st: st, owner: owner, libDir: libDir, thumbDir: thumbDir}
}

// seed is the spec for one seeded asset. Zero values default to a 100x100 image
// indexed "now"; content (written under libDir) is needed for download/zip and
// thumb (written under thumbDir) for /thumb.
type seed struct {
	id, name      string
	kind          domain.AssetKind
	width, height int
	rating        int
	indexed       time.Time
	content       []byte
	thumb         []byte
	display       string
}

func (e *testEnv) add(s seed) domain.AssetID {
	e.t.Helper()
	if s.kind == "" {
		s.kind = domain.KindImage
	}
	if s.width == 0 {
		s.width = 100
	}
	if s.height == 0 {
		s.height = 100
	}
	if s.indexed.IsZero() {
		s.indexed = time.Now().UTC().Truncate(time.Second)
	}
	aid, err := domain.NewAssetID(s.id)
	if err != nil {
		e.t.Fatal(err)
	}
	thumbPath := ""
	if s.thumb != nil {
		thumbPath = filepath.Join(e.thumbDir, s.id+".png")
		if err := os.WriteFile(thumbPath, s.thumb, 0o644); err != nil {
			e.t.Fatal(err)
		}
	}
	if s.content != nil {
		if err := os.WriteFile(filepath.Join(e.libDir, s.name), s.content, 0o644); err != nil {
			e.t.Fatal(err)
		}
	}
	if err := e.st.UpsertAsset(e.ctx, domain.Asset{
		ID: aid, Owner: e.owner, Kind: s.kind, Provider: "local",
		StoragePath: s.name, Name: s.name, Ext: strings.TrimPrefix(filepath.Ext(s.name), "."),
		Size: int64(len(s.content)), Hash: "h-" + s.id, ThumbPath: thumbPath,
		Width: s.width, Height: s.height, Rating: s.rating, DisplayName: s.display,
		CreatedAt: s.indexed, IndexedAt: s.indexed,
	}); err != nil {
		e.t.Fatalf("seed %s: %v", s.id, err)
	}
	return aid
}

func (e *testEnv) newCollection(id, name string) string {
	e.t.Helper()
	cid, err := domain.NewCollectionID(id)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.CreateCollection(e.ctx, domain.Collection{ID: cid, Owner: e.owner, Name: name}); err != nil {
		e.t.Fatalf("create collection: %v", err)
	}
	return id
}

func (e *testEnv) addToCollection(cid string, aid domain.AssetID) {
	e.t.Helper()
	id, err := domain.NewCollectionID(cid)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.AddCollectionItem(e.ctx, e.owner, id, aid); err != nil {
		e.t.Fatalf("add collection item: %v", err)
	}
}

func (e *testEnv) get(path string) *http.Response {
	e.t.Helper()
	resp, err := http.Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func (e *testEnv) getDTOs(path string) []wallpaperDTO {
	e.t.Helper()
	resp := e.get(path)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("GET %s = %d, want 200", path, resp.StatusCode)
	}
	var out []wallpaperDTO
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		e.t.Fatalf("decode %s: %v", path, err)
	}
	return out
}

func dtoIDs(dtos []wallpaperDTO) []string {
	ids := make([]string, len(dtos))
	for i, d := range dtos {
		ids[i] = d.ID
	}
	return ids
}

// decodeJSON decodes an OK response body into dst, failing the test otherwise.
func decodeJSON(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
