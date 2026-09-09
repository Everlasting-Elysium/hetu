package domain

// AssetShape is the coarse aspect-ratio bucket derived from an asset's
// width/height (issue #101), mirroring the AssetKind enum pattern (asset.go):
// a fixed whitelist that gates the ?shape= query param before it reaches SQL.
type AssetShape string

const (
	ShapeLandscape AssetShape = "landscape"
	ShapePortrait  AssetShape = "portrait"
	ShapeSquare    AssetShape = "square"
)

// AllShapes lists every AssetShape in display order. It backs the shape facet
// and the ?shape= whitelist so new shapes are added in exactly one place.
var AllShapes = []AssetShape{ShapeLandscape, ShapePortrait, ShapeSquare}

// ValidShape reports whether s is a known AssetShape, mirroring ValidKind: the
// whitelist behind ?shape= parsing so only enum values reach the SQL layer.
func ValidShape(s string) bool {
	for _, sh := range AllShapes {
		if AssetShape(s) == sh {
			return true
		}
	}
	return false
}

// Aspect-ratio thresholds for the shape bucket, where ratio = width/height.
// landscape: ratio >= ShapeLandscapeMinRatio; portrait: ratio <=
// ShapePortraitMaxRatio; square: strictly between the two. Fixed design values
// from issue #101 (not derived at runtime — portrait is close to, but not
// exactly, the landscape threshold's reciprocal: 1/1.1 ≈ 0.909 vs 0.9 here).
// Exported so internal/store's SQL construction binds these same constants
// instead of re-declaring the magic numbers (single source of truth).
const (
	ShapeLandscapeMinRatio = 1.1
	ShapePortraitMaxRatio  = 0.9
)
