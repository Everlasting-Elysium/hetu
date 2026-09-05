package dam_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

type assetItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Rating   int    `json:"rating"`
	FolderID string `json:"folder_id"`
}

func TestListAssetsFilterAPI(t *testing.T) {
	srv, owner, st := newTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	mk := func(id, name string) domain.AssetID {
		aid, err := domain.NewAssetID(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpsertAsset(ctx, domain.Asset{
			ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
			StoragePath: name, Name: name, Ext: "png", Size: 1, Hash: "h-" + id,
			CreatedAt: now, IndexedAt: now,
		}); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
		return aid
	}
	a1 := mk("a1", "one.png")
	a2 := mk("a2", "two.png")
	_ = mk("a3", "three.png")
	if err := st.BatchMoveToFolder(ctx, owner, []domain.AssetID{a1, a2}, "fx"); err != nil {
		t.Fatal(err)
	}
	if err := st.BatchUpdateRating(ctx, owner, []domain.AssetID{a2}, 5); err != nil {
		t.Fatal(err)
	}

	get := func(query string) []assetItem {
		resp, err := http.Get(srv.URL + "/api/dam/assets" + query)
		if err != nil {
			t.Fatalf("GET %s: %v", query, err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", query, resp.StatusCode)
		}
		var out []assetItem
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	if got := get(""); len(got) != 3 {
		t.Errorf("no filter = %d, want 3", len(got))
	}
	if got := get("?folder=fx"); len(got) != 2 {
		t.Errorf("folder=fx = %d, want 2", len(got))
	}
	if got := get("?rating=5"); len(got) != 1 || got[0].Name != "two.png" {
		t.Errorf("rating=5 = %+v, want [two.png]", got)
	}
	if got := get("?folder=fx&rating=5"); len(got) != 1 {
		t.Errorf("folder=fx&rating=5 = %d, want 1", len(got))
	}
	if got := get("?folder=none"); len(got) != 0 {
		t.Errorf("folder=none = %d, want 0", len(got))
	}
}
