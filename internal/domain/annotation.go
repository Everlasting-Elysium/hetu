package domain

import "time"

// Layer identifies the source tier of an annotation. Higher-priority layers are
// never overwritten by lower ones: manual (user) > ai (model) > extracted (file
// contents / derived). See docs/ai-and-3d.md for the layering rules.
type Layer string

const (
	LayerManual    Layer = "manual"
	LayerAI        Layer = "ai"
	LayerExtracted Layer = "extracted"
)

// AnnotationKey names well-known extracted annotations produced by hetu itself.
const (
	KeyPalette  = "palette"  // JSON array of {hex,weight}, dominant-first
	KeyDominant = "dominant" // JSON string of the dominant "#rrggbb"
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

// ColorMatch is a color-search hit: the asset plus the palette swatch closest to
// the query color and its CIEDE2000 distance. Results are ordered by ascending
// Distance (nearest first).
type ColorMatch struct {
	Asset    Asset
	Hex      string
	Distance float64
}
