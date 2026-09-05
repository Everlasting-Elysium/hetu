package domain

import "fmt"

// TagID identifies a hierarchical label.
type TagID struct{ raw string }

// NewTagID parses s into a TagID.
func NewTagID(s string) (TagID, error) {
	if s == "" {
		return TagID{}, fmt.Errorf("tag id: %w", ErrEmptyID)
	}
	return TagID{raw: s}, nil
}

// String returns the raw tag id.
func (id TagID) String() string { return id.raw }

// Tag is a hierarchical, colorable label that can be attached to assets. Tags
// nest via ParentID (empty = top level).
type Tag struct {
	ID       TagID
	Owner    OwnerID
	ParentID string
	Name     string
	Color    string
}
