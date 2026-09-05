package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store"
)

// mustOpen opens a fresh temp-dir SQLite store with an ensured owner.
func mustOpen(t *testing.T) (context.Context, *store.SQLite, domain.OwnerID) {
	t.Helper()
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
	return ctx, st, owner
}

// seedAsset upserts a minimal image asset and returns its id.
func seedAsset(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id, path string) domain.AssetID {
	t.Helper()
	aid, err := domain.NewAssetID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	a := domain.Asset{
		ID: aid, Owner: owner, Kind: domain.KindImage, Provider: "local",
		StoragePath: path, Name: path, Ext: "png", Size: 1, Hash: "h-" + id,
		CreatedAt: now, IndexedAt: now,
	}
	if err := st.UpsertAsset(ctx, a); err != nil {
		t.Fatalf("seed asset %s: %v", id, err)
	}
	return aid
}

// getAsset fetches an asset, failing the test if it is missing.
func getAsset(t *testing.T, ctx context.Context, st *store.SQLite, owner domain.OwnerID, id domain.AssetID) domain.Asset {
	t.Helper()
	a, err := st.GetAsset(ctx, owner, id)
	if err != nil {
		t.Fatalf("get asset %s: %v", id, err)
	}
	return a
}
