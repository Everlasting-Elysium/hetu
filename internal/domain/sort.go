package domain

// AssetSort selects the ORDER BY applied by ListAssetsFiltered (issue #114),
// mirroring the AssetShape whitelist pattern (shape.go): a fixed enum mapped to
// a hardcoded ORDER BY fragment in the store layer, never raw string
// concatenation, so ?sort= is injection-safe. The zero value ("") preserves the
// pre-#114 default (indexed_at DESC), so every existing DAM caller that never
// sets Sort is unaffected.
type AssetSort string

const (
	SortLatest AssetSort = "latest"
	SortRating AssetSort = "rating"
	SortRandom AssetSort = "random"
)

// AllSorts lists every AssetSort in display order. It backs the ?sort=
// whitelist so new sorts are added in exactly one place.
var AllSorts = []AssetSort{SortLatest, SortRating, SortRandom}

// ValidSort reports whether s is a known AssetSort, mirroring ValidShape: the
// whitelist behind ?sort= parsing so only enum values reach the SQL layer.
func ValidSort(s string) bool {
	for _, so := range AllSorts {
		if AssetSort(s) == so {
			return true
		}
	}
	return false
}
