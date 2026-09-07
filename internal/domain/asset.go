package domain

import "time"

// AssetKind is the coarse category of an asset, stored as a string in the DB.
type AssetKind string

const (
	KindImage    AssetKind = "image"
	KindVideo    AssetKind = "video"
	KindAudio    AssetKind = "audio"
	KindModel    AssetKind = "model" // 3D models (obj/fbx/glb/stl/ztl/zpr...)
	KindDocument AssetKind = "document"
	KindOther    AssetKind = "other"
)

// AllKinds lists every AssetKind in display order. It backs the format facet
// and the ?kind= whitelist so new kinds are added in exactly one place.
var AllKinds = []AssetKind{KindImage, KindVideo, KindAudio, KindModel, KindDocument, KindOther}

// ValidKind reports whether s is a known AssetKind. It is the whitelist behind
// ?kind= parsing: only enum values reach the SQL layer, so kind filtering is
// injection-safe even before the query is parameterized.
func ValidKind(s string) bool {
	for _, k := range AllKinds {
		if AssetKind(s) == k {
			return true
		}
	}
	return false
}

// SupportsColorPalette reports whether extracted-color palette and color search
// apply to this kind. Audio is excluded: its thumbnail is a synthetic waveform
// whose colors describe the render style, not the asset, so audio must not carry
// an extracted palette, appear in color search, or show color swatches (#88).
func (k AssetKind) SupportsColorPalette() bool {
	return k != KindAudio
}

// Asset is an indexed resource. Files are indexed in place (referenced by
// StoragePath), never copied into hetu's own storage.
type Asset struct {
	ID          AssetID
	Owner       OwnerID
	Kind        AssetKind
	Provider    string // storage provider name, e.g. "local"
	StoragePath string
	Name        string
	Ext         string
	Size        int64
	Hash        string // content hash for dedup
	ThumbPath   string
	Width       int
	Height      int
	CreatedAt   time.Time
	IndexedAt   time.Time

	// User-managed metadata (DAM batch operations).
	DeletedAt   *time.Time // nil = live; set = soft-deleted (in trash)
	MissingAt   *time.Time // nil = file found; set = file missing from storage
	Rating      int        // 0-5 stars
	Color       string     // color label, e.g. "#FF5733"; empty = none
	DisplayName string     // user-facing rename; empty = use Name
	FolderID    string     // virtual folder; empty = root

	// CurrentVersionID points at the active revision in asset_versions (issue
	// #58); empty means the asset has no explicit versions and this anchor row
	// is the single implicit version. Display fields (ThumbPath/Width/Height)
	// returned by reads already reflect the current version; StoragePath/Hash
	// stay anchored to the originally indexed file.
	CurrentVersionID string
}

// SimilarityMatch pairs an asset with its cosine similarity score to a query
// vector. Similarity is in [-1, 1]; higher means more similar.
type SimilarityMatch struct {
	Asset      Asset
	Similarity float64
}
