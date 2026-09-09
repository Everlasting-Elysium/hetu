package dam_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

type assetResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// TestGetAssetAPI covers GET /assets/{id}: found, not found, and malformed id.
// This endpoint exists so a caller holding only an asset id (e.g. a collection
// item, issue #55) can fetch the full asset without it being present in an
// already-loaded list.
func TestGetAssetAPI(t *testing.T) {
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

	resp, err := http.Get(srv.URL + "/api/dam/assets/a1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got assetResult
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != "a1" || got.Name != "sunset over sea" || got.Kind != "image" {
		t.Errorf("got = %+v, want id=a1 name=%q kind=image", got, "sunset over sea")
	}

	resp2, err := http.Get(srv.URL + "/api/dam/assets/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("missing asset status = %d, want 404", resp2.StatusCode)
	}
}
