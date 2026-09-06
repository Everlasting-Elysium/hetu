// Package importers migrates external DAM libraries (Eagle, Billfish) into hetu.
// Each source (subpackages eagle, billfish) reads its own on-disk format
// strictly read-only and yields a source-agnostic []ImportItem; the Service
// then indexes each file through internal/index and maps the portable metadata
// onto hetu's folders/tags/rating/annotations. See docs/importers.md for the
// per-source field mapping.
package importers

import (
	"context"
	"time"
)

// SourceKind identifies a migration source format.
type SourceKind string

const (
	KindEagle    SourceKind = "eagle"
	KindBillfish SourceKind = "billfish"
)

// NamePath is a hierarchical name as root→leaf segments, used for both tags and
// folders. A flat name has a single segment. The Service ensures every ancestor
// exists (parent-linked) and attaches/assigns the leaf.
type NamePath []string

// ImportItem is the source-agnostic representation of one asset to migrate,
// produced by a Source and consumed by the Service. Every field maps onto
// hetu's model; source fields with no hetu equivalent are dropped and logged by
// the importer (see each source's Each implementation).
type ImportItem struct {
	AbsPath   string     // absolute path to the original media file (read-only)
	Name      string     // original file name including extension
	Tags      []NamePath // hierarchical tags to attach (leaf is attached)
	Folders   []NamePath // folder memberships; Folders[0] is the primary
	Rating    int        // 0-5 stars (clamped on import)
	Note      string     // user note → manual-layer caption (FTS-searchable)
	SourceURL string     // origin URL → extracted-layer "source.url"
	CreatedAt time.Time  // source creation time → asset.created_at
}

// Source streams import items from a migration library. Implementations MUST
// open the source strictly read-only and never modify it.
type Source interface {
	Kind() SourceKind
	// Each streams every live item to fn, stopping on the first error fn
	// returns or a fatal read error. An item whose media file is unreadable is
	// skipped and logged, not fatal.
	Each(ctx context.Context, fn func(ImportItem) error) error
}
