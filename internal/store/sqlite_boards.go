package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// CreateBoard inserts a new board.
func (s *SQLite) CreateBoard(ctx context.Context, b domain.Board) error {
	if err := s.q.CreateBoard(ctx, db.CreateBoardParams{
		ID:        b.ID.String(),
		OwnerID:   b.Owner.String(),
		Name:      b.Name,
		CreatedAt: b.CreatedAt.Unix(),
		UpdatedAt: b.UpdatedAt.Unix(),
	}); err != nil {
		return fmt.Errorf("create board %s: %w", b.Name, err)
	}
	return nil
}

// ListBoards returns the owner's boards, most recently updated first.
func (s *SQLite) ListBoards(ctx context.Context, owner domain.OwnerID) ([]domain.Board, error) {
	rows, err := s.q.ListBoards(ctx, owner.String())
	if err != nil {
		return nil, fmt.Errorf("list boards: %w", err)
	}
	boards := make([]domain.Board, 0, len(rows))
	for _, r := range rows {
		b, err := rowToBoard(r)
		if err != nil {
			return nil, err
		}
		boards = append(boards, b)
	}
	return boards, nil
}

// GetBoard returns a board by id, or domain.ErrNotFound.
func (s *SQLite) GetBoard(ctx context.Context, owner domain.OwnerID, id domain.BoardID) (domain.Board, error) {
	row, err := s.q.GetBoard(ctx, db.GetBoardParams{ID: id.String(), OwnerID: owner.String()})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Board{}, fmt.Errorf("get board %s: %w", id, domain.ErrNotFound)
		}
		return domain.Board{}, fmt.Errorf("get board %s: %w", id, err)
	}
	return rowToBoard(row)
}

// UpdateBoardName renames a board.
func (s *SQLite) UpdateBoardName(ctx context.Context, owner domain.OwnerID, id domain.BoardID, name string) error {
	if err := s.q.UpdateBoardName(ctx, db.UpdateBoardNameParams{
		Name:      name,
		UpdatedAt: time.Now().Unix(),
		ID:        id.String(),
		OwnerID:   owner.String(),
	}); err != nil {
		return fmt.Errorf("update board %s: %w", id, err)
	}
	return nil
}

// DeleteBoard removes a board and all its items.
func (s *SQLite) DeleteBoard(ctx context.Context, owner domain.OwnerID, id domain.BoardID) error {
	if err := s.q.DeleteBoardItemsByBoard(ctx, id.String()); err != nil {
		return fmt.Errorf("delete board items for %s: %w", id, err)
	}
	if err := s.q.DeleteBoard(ctx, db.DeleteBoardParams{ID: id.String(), OwnerID: owner.String()}); err != nil {
		return fmt.Errorf("delete board %s: %w", id, err)
	}
	return nil
}

// AddBoardItem inserts a new item onto a board and touches the board's
// updated_at timestamp. Returns the persisted item.
func (s *SQLite) AddBoardItem(ctx context.Context, item domain.BoardItem) (domain.BoardItem, error) {
	row, err := s.q.CreateBoardItem(ctx, db.CreateBoardItemParams{
		ID:        item.ID.String(),
		BoardID:   item.BoardID.String(),
		AssetID:   item.AssetID.String(),
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
// multi-row updates. Each item's spatial fields are updated in a single tx.
const batchUpdateItemSQL = `
UPDATE board_items
SET x = ?, y = ?, w = ?, h = ?, rotation = ?, z = ?
WHERE id = ? AND board_id = ?`

// BatchUpdateBoardItems updates spatial fields for multiple items in one tx.
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
		if _, err := stmt.ExecContext(ctx, u.X, u.Y, u.W, u.H, u.Rotation, u.Z, u.ID.String(), bid); err != nil {
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

func rowToBoard(r db.Board) (domain.Board, error) {
	id, err := domain.NewBoardID(r.ID)
	if err != nil {
		return domain.Board{}, fmt.Errorf("row board id: %w", err)
	}
	owner, err := domain.NewOwnerID(r.OwnerID)
	if err != nil {
		return domain.Board{}, fmt.Errorf("row board owner: %w", err)
	}
	return domain.Board{
		ID:        id,
		Owner:     owner,
		Name:      r.Name,
		CreatedAt: time.Unix(r.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(r.UpdatedAt, 0).UTC(),
	}, nil
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
	aid, err := domain.NewAssetID(r.AssetID)
	if err != nil {
		return domain.BoardItem{}, fmt.Errorf("row board item asset_id: %w", err)
	}
	return domain.BoardItem{
		ID:        id,
		BoardID:   bid,
		AssetID:   aid,
		X:         r.X,
		Y:         r.Y,
		W:         r.W,
		H:         r.H,
		Rotation:  r.Rotation,
		Z:         int(r.Z),
		CreatedAt: time.Unix(r.CreatedAt, 0).UTC(),
	}, nil
}
