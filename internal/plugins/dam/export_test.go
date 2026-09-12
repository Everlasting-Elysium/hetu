package dam_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/api"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/plugins/dam"
	"github.com/Everlasting-Elysium/hetu/internal/storage/local"
	"github.com/Everlasting-Elysium/hetu/internal/store"
	"github.com/Everlasting-Elysium/hetu/internal/ziputil"
)

// exportEnv is a wired DAM plugin behind an httptest server with a real local
// storage provider and SQLite store, so /batch/export opens genuine file bytes
// (no mocks — the streaming path needs real files).
type exportEnv struct {
	t     *testing.T
	ctx   context.Context
	srv   *httptest.Server
	st    *store.SQLite
	owner domain.OwnerID
	lib   string
}

func newExportServer(t *testing.T) *exportEnv {
	t.Helper()
	ctx := context.Background()
	lib := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "dam.db"))
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
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	k := kernel.New(kernel.Deps{Log: log, Store: st, ThumbDir: t.TempDir(), JobBuffer: 1})
	k.Storage.Register(local.New(lib))
	p := dam.New(owner)
	if err := p.Init(ctx, k); err != nil {
		t.Fatalf("init plugin: %v", err)
	}
	srv := httptest.NewServer(api.NewRouter(k, []kernel.Plugin{p}, fstest.MapFS{}))
	t.Cleanup(srv.Close)
	return &exportEnv{t: t, ctx: ctx, srv: srv, st: st, owner: owner, lib: lib}
}

// seed writes body to name under the library and indexes an image asset for the
// given owner (the owner param lets a test seed a cross-owner asset). display
// sets the optional display name, which the export entry name prefers.
func (e *exportEnv) seed(owner domain.OwnerID, id, name, body, display string) domain.AssetID {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.lib, name), []byte(body), 0o644); err != nil {
		e.t.Fatal(err)
	}
	aid, err := domain.NewAssetID(id)
	if err != nil {
		e.t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := e.st.UpsertAsset(e.ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: local.ProviderName,
		StoragePath: name, Name: name, Ext: "png", Size: int64(len(body)),
		Hash: "h-" + id, Width: 1, Height: 1, DisplayName: display,
		CreatedAt: now, IndexedAt: now,
	}); err != nil {
		e.t.Fatalf("seed %s: %v", id, err)
	}
	return aid
}

// seedVersioned seeds an asset whose anchor file holds anchorBody, then adds a
// current version whose (different) file holds currentBody — exercising the
// version-resolution branch (issue #58): the anchor's storage_path stays put,
// but the current version points elsewhere, so export must serve the version's
// bytes, not the anchor's. Mirrors wallpaper's media_version_test.
func (e *exportEnv) seedVersioned(anchorName, anchorBody, versionName, currentBody string) domain.AssetID {
	e.t.Helper()
	aid := e.seed(e.owner, "ver1", anchorName, anchorBody, "")
	if err := os.WriteFile(filepath.Join(e.lib, versionName), []byte(currentBody), 0o644); err != nil {
		e.t.Fatal(err)
	}
	anchor, err := e.st.GetAsset(e.ctx, e.owner, aid)
	if err != nil {
		e.t.Fatal(err)
	}
	v1, err := domain.NewVersionID("ver1-v1")
	if err != nil {
		e.t.Fatal(err)
	}
	v2, err := domain.NewVersionID("ver1-v2")
	if err != nil {
		e.t.Fatal(err)
	}
	base := domain.AssetVersion{
		ID: v1, AssetID: aid, Owner: e.owner, Provider: anchor.Provider,
		StoragePath: anchor.StoragePath, Hash: anchor.Hash, Size: anchor.Size,
		Width: anchor.Width, Height: anchor.Height, CreatedAt: anchor.CreatedAt,
	}
	newV := domain.AssetVersion{
		ID: v2, AssetID: aid, Owner: e.owner, Provider: local.ProviderName,
		StoragePath: versionName, Hash: "h-ver1-v2", Size: int64(len(currentBody)),
		Width: anchor.Width, Height: anchor.Height, CreatedAt: time.Now().UTC(),
	}
	if _, err := e.st.AddVersion(e.ctx, e.owner, base, newV); err != nil {
		e.t.Fatalf("add version: %v", err)
	}
	return aid
}

// postExport POSTs asset_ids to /batch/export and returns the raw response for
// the caller to assert status / read the zip body.
func (e *exportEnv) postExport(ids []string) *http.Response {
	e.t.Helper()
	payload, err := json.Marshal(map[string]any{"asset_ids": ids})
	if err != nil {
		e.t.Fatal(err)
	}
	resp, err := http.Post(e.srv.URL+"/api/dam/batch/export", "application/json", bytes.NewReader(payload))
	if err != nil {
		e.t.Fatalf("POST /batch/export: %v", err)
	}
	return resp
}

// exportZip POSTs ids, asserts a 200 application/zip attachment, and returns the
// archive entries as name→content.
func (e *exportEnv) exportZip(ids []string) map[string]string {
	e.t.Helper()
	resp := e.postExport(ids)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("export = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		e.t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != `attachment; filename="assets.zip"` {
		e.t.Errorf("Content-Disposition = %q, want assets.zip attachment", cd)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		e.t.Fatalf("read zip: %v", err)
	}
	out := make(map[string]string, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			e.t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			e.t.Fatal(err)
		}
		out[f.Name] = string(data)
	}
	return out
}

// TestBatchExportHappy covers a multi-asset bundle: each entry carries its bytes,
// and the entry name prefers the display name (a1) over the indexed name (a2).
func TestBatchExportHappy(t *testing.T) {
	e := newExportServer(t)
	a1 := e.seed(e.owner, "a1", "one.png", "ONE", "sunset.png")
	a2 := e.seed(e.owner, "a2", "two.png", "TWO", "")

	entries := e.exportZip([]string{a1.String(), a2.String()})
	if len(entries) != 2 {
		t.Fatalf("zip has %d entries, want 2: %v", len(entries), entries)
	}
	if entries["sunset.png"] != "ONE" {
		t.Errorf("entries = %v, want sunset.png=ONE (display name preferred)", entries)
	}
	if entries["two.png"] != "TWO" {
		t.Errorf("entries = %v, want two.png=TWO (indexed name fallback)", entries)
	}
}

// TestBatchExportSkipsMissing covers a request mixing a valid and an unknown
// (well-formed but non-existent) id: the unknown one is dropped, the valid one
// still ships.
func TestBatchExportSkipsMissing(t *testing.T) {
	e := newExportServer(t)
	a1 := e.seed(e.owner, "a1", "one.png", "ONE", "")

	entries := e.exportZip([]string{a1.String(), "does-not-exist"})
	if len(entries) != 1 || entries["one.png"] != "ONE" {
		t.Errorf("entries = %v, want just one.png=ONE", entries)
	}
}

// TestBatchExportSkipsCrossOwner is the security assertion: another owner's asset
// (with a real, openable backing file under the same library) must never appear
// in the zip, because GetAsset is owner-scoped. The file existing on disk proves
// the guard is the owner check, not a missing file.
func TestBatchExportSkipsCrossOwner(t *testing.T) {
	e := newExportServer(t)
	mine := e.seed(e.owner, "mine", "mine.png", "MINE", "")

	other, err := domain.NewOwnerID("intruder")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.EnsureOwner(e.ctx, other); err != nil {
		t.Fatalf("ensure other owner: %v", err)
	}
	theirs := e.seed(other, "theirs", "theirs.png", "THEIRS", "")

	entries := e.exportZip([]string{mine.String(), theirs.String()})
	if len(entries) != 1 {
		t.Fatalf("zip has %d entries, want 1 (cross-owner skipped): %v", len(entries), entries)
	}
	if _, leaked := entries["theirs.png"]; leaked {
		t.Errorf("cross-owner asset leaked into zip: %v", entries)
	}
	if entries["mine.png"] != "MINE" {
		t.Errorf("entries = %v, want mine.png=MINE", entries)
	}
}

// TestBatchExportServesCurrentVersion covers the version-aware branch: with a
// current version present, the entry name comes from the asset (anchor.png) but
// its bytes must be the current version's ("CURRENT"), never the anchor's.
func TestBatchExportServesCurrentVersion(t *testing.T) {
	e := newExportServer(t)
	aid := e.seedVersioned("anchor.png", "ANCHOR", "current.png", "CURRENT")

	entries := e.exportZip([]string{aid.String()})
	if got := entries["anchor.png"]; got != "CURRENT" {
		t.Errorf("zip entry body = %q, want CURRENT (current version, not anchor)", got)
	}
}

// TestBatchExportErrors covers the request-validation branches: an empty list is
// 400, an all-unresolvable list is 404, and an over-cap list is 400.
func TestBatchExportErrors(t *testing.T) {
	e := newExportServer(t)
	e.seed(e.owner, "a1", "one.png", "ONE", "")

	overCap := make([]string, 0, ziputil.MaxItems+1)
	for i := range ziputil.MaxItems + 1 {
		overCap = append(overCap, fmt.Sprintf("a%d", i))
	}

	cases := []struct {
		name string
		ids  []string
		want int
	}{
		{"empty ids", []string{}, http.StatusBadRequest},
		{"all missing", []string{"nope1", "nope2"}, http.StatusNotFound},
		{"over cap", overCap, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := e.postExport(tc.ids)
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tc.want {
				t.Errorf("export %s = %d, want %d", tc.name, resp.StatusCode, tc.want)
			}
		})
	}
}
