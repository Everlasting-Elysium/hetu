package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func mkBoard(t *testing.T, owner domain.OwnerID, id, name string) domain.Board {
	t.Helper()
	bid, err := domain.NewBoardID(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	return domain.Board{ID: bid, Owner: owner, Name: name, CreatedAt: now, UpdatedAt: now}
}

func mkBoardItem(t *testing.T, id, boardID, assetID string, x, y float64, z int) domain.BoardItem {
	t.Helper()
	iid, err := domain.NewBoardItemID(id)
	if err != nil {
		t.Fatal(err)
	}
	bid, err := domain.NewBoardID(boardID)
	if err != nil {
		t.Fatal(err)
	}
	aid, err := domain.NewAssetID(assetID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	return domain.BoardItem{
		ID: iid, BoardID: bid, AssetID: aid,
		X: x, Y: y, W: 200, H: 200, Rotation: 0, Z: z, CreatedAt: now,
	}
}

func TestSQLite_BoardCRUD(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	b1 := mkBoard(t, owner, "b1", "Moodboard")
	b2 := mkBoard(t, owner, "b2", "References")

	if err := st.CreateBoard(ctx, b1); err != nil {
		t.Fatalf("create b1: %v", err)
	}
	if err := st.CreateBoard(ctx, b2); err != nil {
		t.Fatalf("create b2: %v", err)
	}

	boards, err := st.ListBoards(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) != 2 {
		t.Fatalf("boards = %d, want 2", len(boards))
	}

	got, err := st.GetBoard(ctx, owner, b1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Moodboard" {
		t.Fatalf("name = %q, want Moodboard", got.Name)
	}

	if err := st.UpdateBoardName(ctx, owner, b1.ID, "Renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, _ = st.GetBoard(ctx, owner, b1.ID)
	if got.Name != "Renamed" {
		t.Fatalf("name after rename = %q, want Renamed", got.Name)
	}

	if err := st.DeleteBoard(ctx, owner, b1.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = st.GetBoard(ctx, owner, b1.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_BoardItems(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	b := mkBoard(t, owner, "b1", "Board")
	if err := st.CreateBoard(ctx, b); err != nil {
		t.Fatal(err)
	}

	seedAsset(t, ctx, st, owner, "a1", "img1.png")
	seedAsset(t, ctx, st, owner, "a2", "img2.png")

	item1 := mkBoardItem(t, "i1", "b1", "a1", 10, 20, 0)
	item2 := mkBoardItem(t, "i2", "b1", "a2", 100, 200, 1)

	got1, err := st.AddBoardItem(ctx, item1)
	if err != nil {
		t.Fatalf("add item1: %v", err)
	}
	if got1.X != 10 || got1.Y != 20 {
		t.Fatalf("item1 pos = (%v,%v), want (10,20)", got1.X, got1.Y)
	}
	if _, err := st.AddBoardItem(ctx, item2); err != nil {
		t.Fatalf("add item2: %v", err)
	}

	items, err := st.ListBoardItems(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Z != 0 || items[1].Z != 1 {
		t.Fatalf("z order = %d,%d, want 0,1", items[0].Z, items[1].Z)
	}

	// Batch update positions.
	items[0].X = 50
	items[0].Y = 60
	items[1].X = 300
	items[1].Z = 5
	if err := st.BatchUpdateBoardItems(ctx, b.ID, items); err != nil {
		t.Fatalf("batch update: %v", err)
	}
	updated, _ := st.ListBoardItems(ctx, b.ID)
	// After batch update, item[0] z=0, item[1] z=5, so order changes.
	found := false
	for _, it := range updated {
		if it.ID == items[0].ID && it.X == 50 && it.Y == 60 {
			found = true
		}
	}
	if !found {
		t.Fatal("batch update did not persist item[0] position")
	}

	// Delete single item.
	if err := st.DeleteBoardItem(ctx, b.ID, items[0].ID); err != nil {
		t.Fatalf("delete item: %v", err)
	}
	remaining, _ := st.ListBoardItems(ctx, b.ID)
	if len(remaining) != 1 {
		t.Fatalf("after delete items = %d, want 1", len(remaining))
	}
}

func TestSQLite_DeleteBoardCascadesItems(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	b := mkBoard(t, owner, "b1", "Board")
	if err := st.CreateBoard(ctx, b); err != nil {
		t.Fatal(err)
	}
	seedAsset(t, ctx, st, owner, "a1", "img.png")
	if _, err := st.AddBoardItem(ctx, mkBoardItem(t, "i1", "b1", "a1", 0, 0, 0)); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteBoard(ctx, owner, b.ID); err != nil {
		t.Fatal(err)
	}
	items, _ := st.ListBoardItems(ctx, b.ID)
	if len(items) != 0 {
		t.Fatalf("items after board delete = %d, want 0", len(items))
	}
}
