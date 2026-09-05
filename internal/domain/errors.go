// Package domain holds core value types shared across the kernel and plugins.
// Types here have no I/O and no dependencies on other internal packages.
package domain

import "errors"

var (
	// ErrEmptyID is returned when constructing an ID from an empty string.
	ErrEmptyID = errors.New("domain: empty id")
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("domain: not found")
	// ErrUnsupported is returned when no handler supports an asset type.
	ErrUnsupported = errors.New("domain: unsupported asset type")
	// ErrNoThumbnail is returned by a handler that cannot produce a thumbnail.
	ErrNoThumbnail = errors.New("domain: no thumbnail available")
)
