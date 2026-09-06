package domain

import (
	"fmt"
	"time"
)

// BoardID identifies a moodboard.
type BoardID struct{ raw string }

// NewBoardID parses s into a BoardID.
func NewBoardID(s string) (BoardID, error) {
	if s == "" {
		return BoardID{}, fmt.Errorf("board id: %w", ErrEmptyID)
	}
	return BoardID{raw: s}, nil
}

// String returns the raw board id.
func (id BoardID) String() string { return id.raw }

// BoardItemID identifies a placed item on a board.
type BoardItemID struct{ raw string }

// NewBoardItemID parses s into a BoardItemID.
func NewBoardItemID(s string) (BoardItemID, error) {
	if s == "" {
		return BoardItemID{}, fmt.Errorf("board item id: %w", ErrEmptyID)
	}
	return BoardItemID{raw: s}, nil
}

// String returns the raw board item id.
func (id BoardItemID) String() string { return id.raw }

// Board is a moodboard / infinite canvas containing positioned asset items.
type Board struct {
	ID        BoardID
	Owner     OwnerID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// BoardItem places an asset on a board at a specific position, size, rotation,
// and z-order. All spatial values are floating-point canvas units.
type BoardItem struct {
	ID        BoardItemID
	BoardID   BoardID
	AssetID   AssetID
	X         float64
	Y         float64
	W         float64
	H         float64
	Rotation  float64
	Z         int
	CreatedAt time.Time
}
