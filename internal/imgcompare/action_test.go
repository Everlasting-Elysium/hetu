package imgcompare_test

import (
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/imgcompare"
)

// TestActionDiff exercises the pose-keyword filter and the three-way diff with
// data in every bucket. Only pose-related tags survive the filter: "standing"
// (stand), "arm_up" (arm), "sitting" (sit); "1girl", "smile", "tree" are dropped.
func TestActionDiff(t *testing.T) {
	ref := map[string]float64{"standing": 0.9, "1girl": 0.99, "arm_up": 0.7, "smile": 0.5}
	tgt := map[string]float64{"sitting": 0.8, "1girl": 0.95, "arm_up": 0.6, "tree": 0.4}

	res := imgcompare.Action(ref, tgt)

	if !sameStrings(res.Common, []string{"arm_up"}) {
		t.Fatalf("common = %v, want [arm_up]", res.Common)
	}
	if !sameStrings(res.Missing, []string{"standing"}) {
		t.Fatalf("missing = %v, want [standing]", res.Missing)
	}
	if !sameStrings(res.Extra, []string{"sitting"}) {
		t.Fatalf("extra = %v, want [sitting]", res.Extra)
	}
	// Jaccard = |common| / |union| = 1 / 3.
	if !approx(res.Score, 100.0/3.0, 1e-9) {
		t.Fatalf("Score = %.6f, want %.6f", res.Score, 100.0/3.0)
	}
}

// TestActionIdentical: identical pose tags -> full overlap -> 100.
func TestActionIdentical(t *testing.T) {
	tags := map[string]float64{"standing": 0.9, "arm_up": 0.7, "cat": 0.5}
	res := imgcompare.Action(tags, tags)
	if !approx(res.Score, 100, 1e-9) {
		t.Fatalf("Action(identical).Score = %v, want 100", res.Score)
	}
	if !sameStrings(res.Common, []string{"arm_up", "standing"}) {
		t.Fatalf("common = %v, want [arm_up standing]", res.Common)
	}
}

// TestActionNoPoseTags: when neither side has a pose tag the sets are empty, and
// an empty-vs-empty diff scores 100 (no detectable pose difference).
func TestActionNoPoseTags(t *testing.T) {
	res := imgcompare.Action(map[string]float64{"cat": 0.9}, map[string]float64{"dog": 0.8})
	if !approx(res.Score, 100, 1e-9) {
		t.Fatalf("Score = %v, want 100 (no pose tags either side)", res.Score)
	}
	if len(res.Common) != 0 || len(res.Missing) != 0 || len(res.Extra) != 0 {
		t.Fatalf("diff = %+v, want all empty", res)
	}
}

// TestActionCaseInsensitive: keyword matching ignores case.
func TestActionCaseInsensitive(t *testing.T) {
	res := imgcompare.Action(map[string]float64{"STANDING": 0.9}, map[string]float64{"Standing": 0.8})
	if !sameStrings(res.Missing, []string{"STANDING"}) || !sameStrings(res.Extra, []string{"Standing"}) {
		t.Fatalf("case-different pose tags should still be filtered in: %+v", res)
	}
}
