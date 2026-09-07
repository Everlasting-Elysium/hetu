package store_test

import (
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func TestSQLite_BoardNoteItem(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	b := mkBoard(t, owner, "b1", "Board")
	if err := st.CreateBoard(ctx, b); err != nil {
		t.Fatal(err)
	}

	note := mkBoardNoteItem(t, "n1", "b1", "Hello", 5, 6, 0)
	got, err := st.AddBoardItem(ctx, note)
	if err != nil {
		t.Fatalf("add note: %v", err)
	}
	if got.Kind != domain.BoardItemNote {
		t.Fatalf("kind = %q, want note", got.Kind)
	}
	if got.Text != "Hello" {
		t.Fatalf("text = %q, want Hello", got.Text)
	}
	if got.AssetID.String() != "" {
		t.Fatalf("asset_id = %q, want empty", got.AssetID.String())
	}

	items, err := st.ListBoardItems(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Kind != domain.BoardItemNote || items[0].Text != "Hello" {
		t.Fatalf("listed note = %+v", items[0])
	}
	if items[0].AssetID.String() != "" {
		t.Fatalf("listed note asset_id = %q, want empty", items[0].AssetID.String())
	}
}

func TestSQLite_BatchUpdateBoardItemFields(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	b := mkBoard(t, owner, "b1", "Board")
	if err := st.CreateBoard(ctx, b); err != nil {
		t.Fatal(err)
	}
	seedAsset(t, ctx, st, owner, "a1", "img.png")

	if _, err := st.AddBoardItem(ctx, mkBoardItem(t, "i1", "b1", "a1", 0, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddBoardItem(ctx, mkBoardNoteItem(t, "n1", "b1", "Draft", 10, 10, 1)); err != nil {
		t.Fatal(err)
	}

	items, err := st.ListBoardItems(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	frame := int64(1500)
	const view = `{"orbit":[1,2,3]}`
	for i := range items {
		switch items[i].Kind {
		case domain.BoardItemAsset:
			items[i].FrameMS = &frame
			items[i].View = view
		case domain.BoardItemNote:
			items[i].Text = "Final"
		}
	}
	if err := st.BatchUpdateBoardItems(ctx, b.ID, items); err != nil {
		t.Fatalf("batch update: %v", err)
	}

	got, err := st.ListBoardItems(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range got {
		switch it.Kind {
		case domain.BoardItemAsset:
			if it.FrameMS == nil || *it.FrameMS != 1500 {
				t.Fatalf("asset frame_ms = %v, want 1500", it.FrameMS)
			}
			if it.View != view {
				t.Fatalf("asset view = %q, want %q", it.View, view)
			}
		case domain.BoardItemNote:
			if it.Text != "Final" {
				t.Fatalf("note text = %q, want Final", it.Text)
			}
			if it.AssetID.String() != "" {
				t.Fatalf("note asset_id = %q, want empty after update", it.AssetID.String())
			}
		}
	}
}
