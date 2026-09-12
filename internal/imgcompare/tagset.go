package imgcompare

import "sort"

// diffKeys splits the reference and target tag sets into common (present in
// both), missing (in reference only), and extra (in target only). Confidence
// values are ignored — presence is what a diff is about — and every slice is
// sorted so results are deterministic despite Go's random map iteration. Shared
// by the action dimension (over a pose-filtered subset) and the element
// dimension (over the full tag set).
func diffKeys(ref, target map[string]float64) (common, missing, extra []string) {
	for k := range ref {
		if _, ok := target[k]; ok {
			common = append(common, k)
		} else {
			missing = append(missing, k)
		}
	}
	for k := range target {
		if _, ok := ref[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(common)
	sort.Strings(missing)
	sort.Strings(extra)
	return common, missing, extra
}

// jaccard is |common| / |union| over the diff counts: 1 when the two sets are
// identical, 0 when disjoint, and 1 for two empty sets (no tags on either side =
// no detectable difference). It is the set-overlap score both tag dimensions map
// to 0-100.
func jaccard(common, missing, extra []string) float64 {
	union := len(common) + len(missing) + len(extra)
	if union == 0 {
		return 1
	}
	return float64(len(common)) / float64(union)
}
