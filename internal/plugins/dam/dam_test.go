package dam_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/api"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/plugins/dam"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

type searchResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func newTestServer(t *testing.T) (*httptest.Server, domain.OwnerID, kernel.Store) {
	t.Helper()
	ctx := context.Background()
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
	p := dam.New(owner)
	if err := p.Init(ctx, k); err != nil {
		t.Fatalf("init plugin: %v", err)
	}
	srv := httptest.NewServer(api.NewRouter(k, []kernel.Plugin{p}))
	t.Cleanup(srv.Close)
	return srv, owner, st
}

func TestSearchAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	aid, err := domain.NewAssetID("a1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAsset(ctx, domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: "sunset.png", Name: "sunset over sea", Ext: "png",
		Size: 1, Hash: "h1", Width: 1, Height: 1, CreatedAt: now, IndexedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Match returns the asset.
	got := getSearch(t, srv.URL+"/api/dam/search?q=sunset", http.StatusOK)
	if len(got) != 1 || got[0].Name != "sunset over sea" {
		t.Errorf("search sunset = %+v, want 1 match", got)
	}

	// Field-qualified match.
	got = getSearch(t, srv.URL+"/api/dam/search?q=name:sunset", http.StatusOK)
	if len(got) != 1 {
		t.Errorf("search name:sunset = %d results, want 1", len(got))
	}

	// No match returns an empty array (not null).
	got = getSearch(t, srv.URL+"/api/dam/search?q=nonexistent", http.StatusOK)
	if len(got) != 0 {
		t.Errorf("search nonexistent = %d results, want 0", len(got))
	}

	// Empty query is a client error.
	resp, err := http.Get(srv.URL + "/api/dam/search?q=")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty query status = %d, want 400", resp.StatusCode)
	}
}

func getSearch(t *testing.T, url string, wantStatus int) []searchResult {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, wantStatus)
	}
	var out []searchResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}
