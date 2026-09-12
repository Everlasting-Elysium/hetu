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
	// ErrInvalidQuery is returned when a search query is malformed (e.g. it
	// produces an invalid FTS5 MATCH expression). Callers map it to HTTP 400.
	ErrInvalidQuery = errors.New("domain: invalid query")
	// ErrCollectionItemsMismatch is returned when a reorder request's asset set
	// does not match the collection's current members exactly (differing count,
	// unknown member, or a duplicate). Callers map it to HTTP 400.
	ErrCollectionItemsMismatch = errors.New("domain: collection items mismatch")
	// ErrCollectionCycle is returned when a collection's parent_id would make it
	// its own ancestor (self-reference or a multi-step cycle). Callers map it to
	// HTTP 400.
	ErrCollectionCycle = errors.New("domain: collection parent cycle")
	// ErrSameTag is returned when a tag merge or batch replace names the same
	// tag as both source and target. For merge it would delete a tag into
	// itself; for replace it would delete the tag from the selected assets
	// (INSERT OR IGNORE self-copies, then the DELETE strips it). Callers map it
	// to HTTP 400.
	ErrSameTag = errors.New("domain: source and target tag are the same")
)
