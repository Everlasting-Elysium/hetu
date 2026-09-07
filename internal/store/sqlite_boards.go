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
