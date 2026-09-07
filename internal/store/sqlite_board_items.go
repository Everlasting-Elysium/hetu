package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// AddBoardItem inserts a new item onto a board and touches the board's
// updated_at timestamp. Returns the persisted item.
func (s *SQLite) AddBoardItem(ctx context.Context, item domain.BoardItem) (domain.BoardItem, error) {
	row, err := s.q.CreateBoardItem(ctx, db.CreateBoardItemParams{
		ID:        item.ID.String(),
		BoardID:   item.BoardID.String(),
		Kind:      string(item.Kind),
		AssetID:   sql.NullString{String: item.AssetID.String(), Valid: item.AssetID.String() != ""},
		Text:      item.Text,
		FrameMs:   nullInt64FromPtr(item.FrameMS),
		View:      item.View,
		X:         item.X,
		Y:         item.Y,
		W:         item.W,
		H:         item.H,
		Rotation:  item.Rotation,
		Z:         int64(item.Z),
		CreatedAt: item.CreatedAt.Unix(),
	})
	if err != nil {
		return domain.BoardItem{}, fmt.Errorf("add board item: %w", err)
	}
	_ = s.q.TouchBoard(ctx, db.TouchBoardParams{
		UpdatedAt: time.Now().Unix(),
		ID:        item.BoardID.String(),
	})
	return rowToBoardItem(row)
}

// BatchAddBoardItems inserts multiple items onto a board in one transaction and
// touches the board's updated_at. Items are pre-validated and de-duplicated by
// the caller (see the /items/batch handler); an empty slice is a no-op so a
// request whose assets were all already on the board does not needlessly touch
// it. Loops the single-row CreateBoardItem via WithTx rather than a dynamic
// multi-row INSERT, which sqlc cannot express.
func (s *SQLite) BatchAddBoardItems(ctx context.Context, boardID domain.BoardID, items []domain.BoardItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch add items: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := s.q.WithTx(tx)
	for _, it := range items {
		if _, err := q.CreateBoardItem(ctx, db.CreateBoardItemParams{
			ID:        it.ID.String(),
			BoardID:   it.BoardID.String(),
			Kind:      string(it.Kind),
			AssetID:   sql.NullString{String: it.AssetID.String(), Valid: it.AssetID.String() != ""},
			Text:      it.Text,
			FrameMs:   nullInt64FromPtr(it.FrameMS),
			View:      it.View,
			X:         it.X,
			Y:         it.Y,
			W:         it.W,
			H:         it.H,
			Rotation:  it.Rotation,
			Z:         int64(it.Z),
			CreatedAt: it.CreatedAt.Unix(),
		}); err != nil {
			return fmt.Errorf("add board item %s: %w", it.ID, err)
		}
	}
	if err := q.TouchBoard(ctx, db.TouchBoardParams{UpdatedAt: time.Now().Unix(), ID: boardID.String()}); err != nil {
		return fmt.Errorf("touch board %s: %w", boardID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch add items: %w", err)
	}
	return nil
}

// ListBoardItems returns all items on a board, ordered by z then created_at.
func (s *SQLite) ListBoardItems(ctx context.Context, boardID domain.BoardID) ([]domain.BoardItem, error) {
	rows, err := s.q.ListBoardItems(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("list board items: %w", err)
	}
	items := make([]domain.BoardItem, 0, len(rows))
	for _, r := range rows {
		it, err := rowToBoardItem(r)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

// batchUpdateItemSQL is hand-written because sqlc does not support dynamic
// multi-row updates. Each item's kind, asset_id, text, frame_ms, view, and
// spatial fields (x/y/w/h/rotation/z) are updated in a single tx.
const batchUpdateItemSQL = `
UPDATE board_items
SET kind = ?, asset_id = ?, text = ?, frame_ms = ?, view = ?, x = ?, y = ?, w = ?, h = ?, rotation = ?, z = ?
WHERE id = ? AND board_id = ?`

// BatchUpdateBoardItems updates all mutable item fields (kind, asset_id, text,
// frame_ms, view, x, y, w, h, rotation, z) for multiple items in one tx.
func (s *SQLite) BatchUpdateBoardItems(ctx context.Context, boardID domain.BoardID, updates []domain.BoardItem) error {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch update items: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, batchUpdateItemSQL)
	if err != nil {
		return fmt.Errorf("prepare batch update items: %w", err)
	}
	defer stmt.Close()

	bid := boardID.String()
	for _, u := range updates {
		aid := sql.NullString{String: u.AssetID.String(), Valid: u.AssetID.String() != ""}
		if _, err := stmt.ExecContext(ctx, string(u.Kind), aid, u.Text, nullInt64FromPtr(u.FrameMS), u.View,
			u.X, u.Y, u.W, u.H, u.Rotation, u.Z, u.ID.String(), bid); err != nil {
			return fmt.Errorf("update item %s: %w", u.ID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE boards SET updated_at = ? WHERE id = ?", time.Now().Unix(), bid); err != nil {
		return fmt.Errorf("touch board %s: %w", boardID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch update items: %w", err)
	}
	return nil
}

// DeleteBoardItem removes a single item from a board.
func (s *SQLite) DeleteBoardItem(ctx context.Context, boardID domain.BoardID, itemID domain.BoardItemID) error {
	if err := s.q.DeleteBoardItem(ctx, db.DeleteBoardItemParams{
		ID:      itemID.String(),
		BoardID: boardID.String(),
	}); err != nil {
		return fmt.Errorf("delete board item %s: %w", itemID, err)
	}
	return nil
}

func rowToBoardItem(r db.BoardItem) (domain.BoardItem, error) {
	id, err := domain.NewBoardItemID(r.ID)
	if err != nil {
		return domain.BoardItem{}, fmt.Errorf("row board item id: %w", err)
	}
	bid, err := domain.NewBoardID(r.BoardID)
	if err != nil {
		return domain.BoardItem{}, fmt.Errorf("row board item board_id: %w", err)
	}
	var aid domain.AssetID // zero value for note items (asset_id is NULL)
	if r.AssetID.Valid && r.AssetID.String != "" {
		aid, err = domain.NewAssetID(r.AssetID.String)
		if err != nil {
			return domain.BoardItem{}, fmt.Errorf("row board item asset_id: %w", err)
		}
	}
	var frameMS *int64
	if r.FrameMs.Valid {
		v := r.FrameMs.Int64
		frameMS = &v
	}
	return domain.BoardItem{
		ID:        id,
		BoardID:   bid,
		Kind:      domain.BoardItemKind(r.Kind),
		AssetID:   aid,
		Text:      r.Text,
		FrameMS:   frameMS,
		View:      r.View,
		X:         r.X,
		Y:         r.Y,
		W:         r.W,
		H:         r.H,
		Rotation:  r.Rotation,
		Z:         int(r.Z),
		CreatedAt: time.Unix(r.CreatedAt, 0).UTC(),
	}, nil
}

// nullInt64FromPtr converts an optional int64 into sql.NullInt64 for storage:
// a nil pointer becomes NULL, a set pointer becomes a valid value.
func nullInt64FromPtr(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}
