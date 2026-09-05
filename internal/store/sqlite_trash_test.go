package store_test

import (
	"errors"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func TestSQLite_TrashRestoreAndList(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")
	seedAsset(t, ctx, st, owner, "a3", "3.png")

	if err := st.BatchTrashAssets(ctx, owner, []domain.AssetID{a1, a2}); err != nil {
		t.Fatalf("trash: %v", err)
	}

	live, err := st.ListAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 || live[0].ID != mustAssetID(t, "a3") {
		t.Fatalf("live = %d assets, want 1 (a3)", len(live))
	}

	trashed, err := st.ListTrashedAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 2 {
		t.Fatalf("trashed = %d, want 2", len(trashed))
	}
	if trashed[0].DeletedAt == nil {
		t.Fatal("trashed asset DeletedAt is nil, want a timestamp")
	}

	if err := st.BatchRestoreAssets(ctx, owner, []domain.AssetID{a1, a2}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	live, err = st.ListAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 3 {
		t.Fatalf("after restore live = %d, want 3", len(live))
	}
	trashed, err = st.ListTrashedAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 0 {
		t.Fatalf("after restore trashed = %d, want 0", len(trashed))
	}
}

func TestSQLite_PurgeTrash(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")

	if err := st.BatchTrashAssets(ctx, owner, []domain.AssetID{a1, a2}); err != nil {
		t.Fatalf("trash: %v", err)
	}
	// retentionDays=0 empties the trash entirely.
	if err := st.PurgeTrash(ctx, owner, 0); err != nil {
		t.Fatalf("purge: %v", err)
	}
	trashed, err := st.ListTrashedAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 0 {
		t.Fatalf("after purge trashed = %d, want 0", len(trashed))
	}
	// Purged rows are gone entirely, not merely restored.
	if _, err := st.GetAsset(ctx, owner, a1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("purged asset err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_PurgeTrash_KeepsRecent(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")

	if err := st.BatchTrashAssets(ctx, owner, []domain.AssetID{a1}); err != nil {
		t.Fatalf("trash: %v", err)
	}
	// A 30-day retention must not purge something trashed just now.
	if err := st.PurgeTrash(ctx, owner, 30); err != nil {
		t.Fatalf("purge: %v", err)
	}
	trashed, err := st.ListTrashedAssets(ctx, owner, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 1 {
		t.Fatalf("trashed = %d, want 1 (still within retention)", len(trashed))
	}
}

func mustAssetID(t *testing.T, s string) domain.AssetID {
	t.Helper()
	id, err := domain.NewAssetID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
