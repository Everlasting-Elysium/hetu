package imgcompare_test

import (
	"math"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/imgcompare"
)

// colorScale mirrors imgcompare's colorDeltaEScale so the tests can reproduce the
// documented score formula 100*exp(-ΔE/scale) independently.
const colorScale = 7.0

func TestColorIdentical(t *testing.T) {
	img := solid(16, 16, color.RGB{R: 200, G: 100, B: 50})
	res := imgcompare.Color(img, img)
	if !approx(res.Score, 100, 1e-9) {
		t.Fatalf("Color(identical).Score = %v, want 100", res.Score)
	}
	if !approx(res.DominantDeltaE, 0, 1e-9) || !approx(res.AverageDeltaE, 0, 1e-9) {
		t.Fatalf("Color(identical) ΔE = dom %v avg %v, want 0", res.DominantDeltaE, res.AverageDeltaE)
	}
	if len(res.RefPalette) != 1 || len(res.Matches) != 1 || !approx(res.Matches[0].DeltaE, 0, 1e-9) {
		t.Fatalf("Color(identical) palette/matches = %+v / %+v", res.RefPalette, res.Matches)
	}
}

// TestColorRedVsBlue pins the color dimension against fixed-reference values: the
// dominant/average ΔE00 must equal color.Distance computed independently over the
// pure-red and pure-blue Lab values, and the score must follow the documented
// exp map. Pure red vs pure blue is a huge perceptual gap, so the score is ~0.
func TestColorRedVsBlue(t *testing.T) {
	red, blue := color.RGB{R: 255}, color.RGB{B: 255}
	wantDeltaE := color.Distance(red.Lab(), blue.Lab())

	res := imgcompare.Color(solid(16, 16, red), solid(16, 16, blue))

	if !approx(res.DominantDeltaE, wantDeltaE, 1e-9) {
		t.Fatalf("DominantDeltaE = %.6f, want %.6f", res.DominantDeltaE, wantDeltaE)
	}
	if !approx(res.AverageDeltaE, wantDeltaE, 1e-9) {
		t.Fatalf("AverageDeltaE = %.6f, want %.6f", res.AverageDeltaE, wantDeltaE)
	}
	wantScore := 100 * math.Exp(-wantDeltaE/colorScale)
	if !approx(res.Score, wantScore, 1e-6) {
		t.Fatalf("Score = %.6f, want %.6f (ΔE00=%.4f)", res.Score, wantScore, wantDeltaE)
	}
	// Sanity band: red vs blue (ΔE00 ~52) must collapse to a near-zero score.
	if res.Score < 0 || res.Score > 2 {
		t.Fatalf("Score = %.4f, want within (0,2] for red vs blue (ΔE00=%.4f)", res.Score, wantDeltaE)
	}

	// Warmth and chroma deltas must match the same primitives, single-swatch.
	rl, bl := red.Lab(), blue.Lab()
	if !approx(res.WarmthShift, bl.A-rl.A, 1e-9) {
		t.Fatalf("WarmthShift = %.6f, want %.6f", res.WarmthShift, bl.A-rl.A)
	}
	wantChroma := math.Hypot(bl.A, bl.B) - math.Hypot(rl.A, rl.B)
	if !approx(res.ChromaDiff, wantChroma, 1e-9) {
		t.Fatalf("ChromaDiff = %.6f, want %.6f", res.ChromaDiff, wantChroma)
	}
	if len(res.Matches) != 1 || !approx(res.Matches[0].DeltaE, wantDeltaE, 1e-9) {
		t.Fatalf("Matches = %+v, want single match ΔE=%.4f", res.Matches, wantDeltaE)
	}
}

// TestColorMidRange checks a moderate difference (pure red vs a muted red) lands
// in a plausible mid band and follows the exp map exactly.
func TestColorMidRange(t *testing.T) {
	red, muted := color.RGB{R: 255}, color.RGB{R: 200, G: 60, B: 60}
	wantDeltaE := color.Distance(red.Lab(), muted.Lab())

	res := imgcompare.Color(solid(16, 16, red), solid(16, 16, muted))

	if !approx(res.DominantDeltaE, wantDeltaE, 1e-9) {
		t.Fatalf("DominantDeltaE = %.6f, want %.6f", res.DominantDeltaE, wantDeltaE)
	}
	wantScore := 100 * math.Exp(-wantDeltaE/colorScale)
	if !approx(res.Score, wantScore, 1e-6) {
		t.Fatalf("Score = %.6f, want %.6f", res.Score, wantScore)
	}
	if res.Score <= 2 || res.Score >= 100 {
		t.Fatalf("Score = %.4f, want a mid value in (2,100) (ΔE00=%.4f)", res.Score, wantDeltaE)
	}
}
