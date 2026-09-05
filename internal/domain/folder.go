package domain

import "fmt"

// FolderID identifies a virtual folder.
type FolderID struct{ raw string }

// NewFolderID parses s into a FolderID.
func NewFolderID(s string) (FolderID, error) {
	if s == "" {
		return FolderID{}, fmt.Errorf("folder id: %w", ErrEmptyID)
	}
	return FolderID{raw: s}, nil
}

// String returns the raw folder id.
func (id FolderID) String() string { return id.raw }

// Folder is a virtual organization node for the DAM plugin, decoupled from the
// filesystem. Path is the redundant root-to-node path stored for fast lookup.
type Folder struct {
	ID       FolderID
	Owner    OwnerID
	ParentID string
	Name     string
	Path     string
}
