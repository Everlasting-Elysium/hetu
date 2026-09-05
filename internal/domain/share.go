package domain

import (
	"fmt"
	"time"
)

// ShareID identifies a share link.
type ShareID struct{ raw string }

// NewShareID parses s into a ShareID.
func NewShareID(s string) (ShareID, error) {
	if s == "" {
		return ShareID{}, fmt.Errorf("share id: %w", ErrEmptyID)
	}
	return ShareID{raw: s}, nil
}

// String returns the raw share id.
func (id ShareID) String() string { return id.raw }

// Share is a shareable link to an asset, folder, or tag with optional expiry,
// password protection, and permission. Validation of TargetType/Permission
// belongs to the share-creation API (see issue #4); the store persists what it
// is given.
type Share struct {
	ID           ShareID
	Owner        OwnerID
	TargetType   string     // "asset" / "folder" / "tag"
	TargetID     string     // id of the shared target
	Token        string     // URL-facing token, unique
	ExpiresAt    *time.Time // nil = never expires
	PasswordHash string     // empty = no password
	Permission   string     // "read"
	CreatedAt    time.Time
}
