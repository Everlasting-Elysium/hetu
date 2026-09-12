package imgcompare

import "strings"

// poseKeywords are the substrings that flag a WD-tagger label as pose/action
// related. Matching is case-insensitive substring containment, a deliberately
// rough heuristic (so "arm" also catches "arms"/"armor", "leg" catches
// "legend"): precise pose analysis is left to a VLM. See ActionResult.
var poseKeywords = []string{
	"stand", "sit", "pose", "action", "run", "jump", "lying", "kneel",
	"arm", "leg", "hand", "walk", "crouch", "squat", "bend", "reach",
	"gesture", "dance", "lean",
}

// ActionResult is the pose/action comparison over the pose-related subset of
// each image's tags. This is a rough keyword heuristic over tag names; precise
// pose/action analysis is left to a VLM.
type ActionResult struct {
	Score   float64  `json:"score"`
	Common  []string `json:"common"`  // pose tags in both
	Missing []string `json:"missing"` // pose tags in reference only
	Extra   []string `json:"extra"`   // pose tags in target only
}

// Action compares the pose-related tags of the reference and target. It filters
// each tag map to pose keywords, diffs the two subsets, and scores their Jaccard
// overlap on 0-100 (100 = identical pose tags, including when neither image has
// any). Pure function with no I/O: the caller supplies tags already fetched from
// the AI sidecar's /tag endpoint.
func Action(refTags, targetTags map[string]float64) ActionResult {
	common, missing, extra := diffKeys(filterPose(refTags), filterPose(targetTags))
	return ActionResult{
		Score:   100 * jaccard(common, missing, extra),
		Common:  common,
		Missing: missing,
		Extra:   extra,
	}
}

// filterPose keeps only the tags whose name contains a pose keyword.
func filterPose(tags map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(tags))
	for name, conf := range tags {
		if isPose(name) {
			out[name] = conf
		}
	}
	return out
}

// isPose reports whether name contains any pose keyword, case-insensitively.
func isPose(name string) bool {
	lower := strings.ToLower(name)
	for _, kw := range poseKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
