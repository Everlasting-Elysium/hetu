package domain

import (
	"fmt"
	"strings"
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

// BoardItemKind discriminates between asset placements and text notes.
type BoardItemKind string

const (
	BoardItemAsset BoardItemKind = "asset"
	BoardItemNote  BoardItemKind = "note"
)

// BoardItem places either an asset or a text note on a board at a specific
// position, size, rotation, and z-order. All spatial values are floating-point
// canvas units.
type BoardItem struct {
	ID        BoardItemID
	BoardID   BoardID
	Kind      BoardItemKind
	AssetID   AssetID // zero value for note items
	Text      string  // non-empty for note items
	FrameMS   *int64  // video frame time in ms, nil when unused
	View      string  // model camera JSON, empty when unused
	X         float64
	Y         float64
	W         float64
	H         float64
	Rotation  float64
	Z         int
	CreatedAt time.Time
}

// maxNoteText caps the length of a note's text to prevent unbounded storage.
const maxNoteText = 10000

// ValidateBoardItem checks kind-dependent invariants: asset items must have a
// non-zero AssetID and empty Text; note items must have a zero AssetID and
// non-empty Text (capped at maxNoteText characters).
func ValidateBoardItem(it BoardItem) error {
	switch it.Kind {
	case BoardItemAsset:
		if it.AssetID.String() == "" {
			return fmt.Errorf("asset item requires asset_id")
		}
		if it.Text != "" {
			return fmt.Errorf("asset item must not have text")
		}
	case BoardItemNote:
		if it.AssetID.String() != "" {
			return fmt.Errorf("note item must not have asset_id")
		}
		if strings.TrimSpace(it.Text) == "" {
			return fmt.Errorf("note item requires text")
		}
		if len(it.Text) > maxNoteText {
			return fmt.Errorf("note text exceeds %d characters", maxNoteText)
		}
	default:
		return fmt.Errorf("unknown board item kind %q", it.Kind)
	}
	return nil
}
