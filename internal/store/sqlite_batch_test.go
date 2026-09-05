package store_test

import (
	"errors"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func TestSQLite_BatchUpdateRating(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")
	a3 := seedAsset(t, ctx, st, owner, "a3", "3.png")
	other := seedAsset(t, ctx, st, owner, "a4", "4.png")

	if err := st.BatchUpdateRating(ctx, owner, []domain.AssetID{a1, a2, a3}, 4); err != nil {
		t.Fatalf("batch rating: %v", err)
	}
	for _, id := range []domain.AssetID{a1, a2, a3} {
		if got := getAsset(t, ctx, st, owner, id); got.Rating != 4 {
			t.Errorf("asset %s rating = %d, want 4", id, got.Rating)
		}
	}
	if got := getAsset(t, ctx, st, owner, other); got.Rating != 0 {
		t.Errorf("excluded asset rating = %d, want 0", got.Rating)
	}
}

func TestSQLite_BatchUpdateColor(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")

	if err := st.BatchUpdateColor(ctx, owner, []domain.AssetID{a1, a2}, "#FF5733"); err != nil {
		t.Fatalf("batch color: %v", err)
	}
	for _, id := range []domain.AssetID{a1, a2} {
		if got := getAsset(t, ctx, st, owner, id); got.Color != "#FF5733" {
			t.Errorf("asset %s color = %q, want #FF5733", id, got.Color)
		}
	}
}

func TestSQLite_BatchUpdateDisplayName(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")

	if err := st.BatchUpdateDisplayName(ctx, owner, []domain.AssetID{a1, a2}, "renamed"); err != nil {
		t.Fatalf("batch display name: %v", err)
	}
	for _, id := range []domain.AssetID{a1, a2} {
		if got := getAsset(t, ctx, st, owner, id); got.DisplayName != "renamed" {
			t.Errorf("asset %s display_name = %q, want renamed", id, got.DisplayName)
		}
	}
}

func TestSQLite_BatchRenameDisplayNames(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")

	renames := map[domain.AssetID]string{a1: "photo_1", a2: "photo_2"}
	if err := st.BatchRenameDisplayNames(ctx, owner, renames); err != nil {
		t.Fatalf("batch rename: %v", err)
	}
	if got := getAsset(t, ctx, st, owner, a1); got.DisplayName != "photo_1" {
		t.Errorf("a1 display_name = %q, want photo_1", got.DisplayName)
	}
	if got := getAsset(t, ctx, st, owner, a2); got.DisplayName != "photo_2" {
		t.Errorf("a2 display_name = %q, want photo_2", got.DisplayName)
	}
}

func TestSQLite_BatchMoveToFolder(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	a1 := seedAsset(t, ctx, st, owner, "a1", "1.png")
	a2 := seedAsset(t, ctx, st, owner, "a2", "2.png")

	fid, err := domain.NewFolderID("fold-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateFolder(ctx, domain.Folder{ID: fid, Owner: owner, Name: "Album", Path: "/Album"}); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := st.BatchMoveToFolder(ctx, owner, []domain.AssetID{a1, a2}, fid.String()); err != nil {
		t.Fatalf("batch move: %v", err)
	}
	for _, id := range []domain.AssetID{a1, a2} {
		if got := getAsset(t, ctx, st, owner, id); got.FolderID != "fold-1" {
			t.Errorf("asset %s folder_id = %q, want fold-1", id, got.FolderID)
		}
	}
}

func TestSQLite_GetAsset_NotFound(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	missing, err := domain.NewAssetID("nope")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetAsset(ctx, owner, missing); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
