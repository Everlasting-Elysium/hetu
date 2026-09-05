package domain

import "time"

// Meta is metadata extracted from an asset by an AssetHandler.
type Meta struct {
	Kind   AssetKind
	Width  int
	Height int
}

// Entry is a single item returned when listing a storage path.
type Entry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// FileInfo describes a single storage object.
type FileInfo struct {
	Size    int64
	IsDir   bool
	ModTime time.Time
}
