package domain

import (
	"strings"
	"time"
)

// Layer identifies the source tier of an annotation. Higher-priority layers are
// never overwritten by lower ones: manual (user) > ai (model) > extracted (file
// contents / derived). See docs/ai-and-3d.md for the layering rules.
type Layer string

const (
	LayerManual    Layer = "manual"
	LayerAI        Layer = "ai"
	LayerExtracted Layer = "extracted"
)

// AnnotationKey names well-known annotations produced by hetu itself.
const (
	// KeyCaption is the ai-layer key holding the sidecar's caption/description
	// (JSON string). Written by the AI tagging pipeline; see docs/ai-and-3d.md.
	KeyCaption = "caption"

	KeyPalette  = "palette"  // JSON array of {hex,weight}, dominant-first
	KeyDominant = "dominant" // JSON string of the dominant "#rrggbb"
	KeyPHash    = "phash"    // JSON string of the perceptual hash (uint64 decimal)

	// EXIF keys (layer=extracted, prefix "exif.").
	KeyExifCameraMake   = "exif.camera_make"   // JSON string
	KeyExifCameraModel  = "exif.camera_model"  // JSON string
	KeyExifLensModel    = "exif.lens_model"    // JSON string
	KeyExifISO          = "exif.iso"           // JSON int
	KeyExifFNumber      = "exif.f_number"      // JSON string, e.g. "f/2.8"
	KeyExifExposure     = "exif.exposure_time"  // JSON string, e.g. "1/125"
	KeyExifFocalLength  = "exif.focal_length"  // JSON string, e.g. "50mm"
	KeyExifGPSLatitude  = "exif.gps_latitude"  // JSON float64
	KeyExifGPSLongitude = "exif.gps_longitude" // JSON float64
	KeyExifDateTime     = "exif.date_time"     // JSON string (RFC 3339)

	// IPTC keys (layer=extracted, prefix "iptc.").
	KeyIPTCKeywords = "iptc.keywords" // JSON []string

	// XMP keys (layer=extracted, prefix "xmp.").
	KeyXMPCreator     = "xmp.creator"     // JSON string
	KeyXMPDescription = "xmp.description" // JSON string
	KeyXMPSubject     = "xmp.subject"     // JSON []string
	KeyXMPCopyright   = "xmp.copyright"   // JSON string
)

// Annotation is one layered metadata key/value for an asset. Value is a
// JSON-serialized payload; Model is set only for the ai layer (empty otherwise).
type Annotation struct {
	AssetID   AssetID
	Layer     Layer
	Key       string
	Value     string
	Model     string
	CreatedAt time.Time
}

// AITagResult is a parsed AI tagging pass ready for non-destructive persistence
// to the ai layer: TagNames are trimmed, de-duplicated, non-empty labels (order
// preserved); Caption may be empty; Model names the producing model so the ai
// layer can be cleared and re-run after a model upgrade. Build it with
// [NewAITagResult] so callers never persist raw, unvalidated sidecar output.
type AITagResult struct {
	TagNames []string
	Caption  string
	Model    string
}

// NewAITagResult parses a sidecar tagging response into an [AITagResult]: it
// trims each name and the caption, drops blank names, and de-duplicates while
// preserving first-seen order (parse-don't-validate).
func NewAITagResult(names []string, caption, model string) AITagResult {
	seen := make(map[string]struct{}, len(names))
	tags := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		tags = append(tags, n)
	}
	return AITagResult{TagNames: tags, Caption: strings.TrimSpace(caption), Model: strings.TrimSpace(model)}
}

// ColorMatch is a color-search hit: the asset plus the palette swatch closest to
// the query color and its CIEDE2000 distance. Results are ordered by ascending
// Distance (nearest first).
type ColorMatch struct {
	Asset    Asset
	Hex      string
	Distance float64
}

// DuplicateGroup is a set of assets sharing the same content hash. The Hash
// field is the SHA-256 that unites them; Assets contains every live member.
type DuplicateGroup struct {
	Hash   string  `json:"hash"`
	Assets []Asset `json:"assets"`
}

// SimilarGroup is a set of assets whose perceptual hashes are within a hamming
// distance threshold. Anchor is the reference asset; Members are the similar
// ones, each annotated with its hamming distance.
type SimilarGroup struct {
	Anchor  Asset         `json:"anchor"`
	Members []SimilarHit  `json:"members"`
}

// SimilarHit pairs an asset with its hamming distance to the group anchor.
type SimilarHit struct {
	Asset    Asset `json:"asset"`
	Distance int   `json:"distance"`
}
