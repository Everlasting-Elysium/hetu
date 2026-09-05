package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

func TestSQLite_UpsertAndList(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	owner, err := domain.NewOwnerID("tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureOwner(ctx, owner); err != nil {
		t.Fatalf("ensure owner: %v", err)
	}

	id, err := domain.NewAssetID("asset-1")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	asset := domain.Asset{
		ID: id, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: "a/b.png", Name: "b.png", Ext: "png", Size: 123,
		Hash: "deadbeef", ThumbPath: "/thumbs/1.jpg", Width: 800, Height: 600,
		CreatedAt: now, IndexedAt: now,
	}
	if err := st.UpsertAsset(ctx, asset); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := st.ListAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Name != "b.png" || got[0].Width != 800 || got[0].Hash != "deadbeef" {
		t.Fatalf("round-trip mismatch: %+v", got[0])
	}

	// Upserting the same (owner, provider, path) updates in place, no duplicate.
	asset.Size = 999
	if err := st.UpsertAsset(ctx, asset); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got, err = st.ListAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("after re-upsert len = %d, want 1", len(got))
	}
	if got[0].Size != 999 {
		t.Fatalf("size = %d, want 999 (updated)", got[0].Size)
	}
}
