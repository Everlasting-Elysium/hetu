package imgcompare

import (
	"image"
	"math"

	"github.com/Everlasting-Elysium/hetu/internal/color"
)

// colorDeltaEScale sets how fast the color score decays with ΔE00 in
// 100*exp(-ΔE/scale). At scale=7: ΔE≈1 (a just-noticeable difference) still
// scores ~87, ΔE≈10 ~24, and ΔE≈20-30 collapses to ~6-1, matching the intuition
// that only near-identical palettes deserve a high score.
const colorDeltaEScale = 7.0

// SwatchMatch pairs one reference swatch with its nearest target swatch by ΔE00,
// so the frontend can draw "this reference color became that target color".
type SwatchMatch struct {
	Ref    color.Swatch `json:"ref"`
	Target color.Swatch `json:"target"`
	DeltaE float64      `json:"delta_e"`
}

// ColorResult is the color-dimension comparison plus every value the frontend
// needs to draw both palettes and the palette-to-palette mapping.
type ColorResult struct {
	Score          float64       `json:"score"`            // 0-100
	RefPalette     color.Palette `json:"ref_palette"`      // dominant-first
	TargetPalette  color.Palette `json:"target_palette"`   // dominant-first
	DominantDeltaE float64       `json:"dominant_delta_e"` // ΔE00 of the two Palette[0]
	AverageDeltaE  float64       `json:"average_delta_e"`  // ΔE00 of the weighted-average colors
	WarmthShift    float64       `json:"warmth_shift"`     // target avg Lab a* minus ref avg a*; >0 = target warmer/redder
	ChromaDiff     float64       `json:"chroma_diff"`      // target avg chroma minus ref avg chroma
	Matches        []SwatchMatch `json:"matches"`          // per reference swatch, its nearest target swatch
}

// Color scores palette similarity between the reference and target images. Every
// color computation is delegated to internal/color (palette extraction, Lab
// conversion, CIEDE2000) — this dimension only arranges those primitives. The
// score is the weight-weighted mean nearest-match ΔE00 mapped through
// 100*exp(-ΔE/colorDeltaEScale): identical palettes score 100, wildly different
// hues (e.g. pure red vs pure blue, ΔE00 ~50) score near 0.
func Color(ref, target image.Image) ColorResult {
	refPal := color.ExtractPalette(ref, color.DefaultSampleMaxDim, color.DefaultPaletteSize)
	tgtPal := color.ExtractPalette(target, color.DefaultSampleMaxDim, color.DefaultPaletteSize)

	res := ColorResult{RefPalette: refPal, TargetPalette: tgtPal}
	if len(refPal) == 0 || len(tgtPal) == 0 {
		return res // no opaque pixels on one side: nothing comparable, score 0
	}

	res.DominantDeltaE = color.Distance(refPal[0].Lab(), tgtPal[0].Lab())

	refAvg, tgtAvg := weightedAverage(refPal).Lab(), weightedAverage(tgtPal).Lab()
	res.AverageDeltaE = color.Distance(refAvg, tgtAvg)
	res.WarmthShift = tgtAvg.A - refAvg.A
	res.ChromaDiff = chroma(tgtAvg) - chroma(refAvg)

	res.Matches = nearestMatches(refPal, tgtPal)
	res.Score = 100 * math.Exp(-weightedMeanDeltaE(res.Matches)/colorDeltaEScale)
	return res
}

// weightedAverage collapses a palette into one RGB, each swatch weighted by its
// coverage. This reuses the already-extracted palette instead of a second
// full-image pass; on a downsampled palette it closely tracks the mean color.
func weightedAverage(pal color.Palette) color.RGB {
	var r, g, b, wsum float64
	for _, s := range pal {
		r += s.Weight * float64(s.R)
		g += s.Weight * float64(s.G)
		b += s.Weight * float64(s.B)
		wsum += s.Weight
	}
	if wsum == 0 {
		return color.RGB{}
	}
	return color.RGB{R: round8(r / wsum), G: round8(g / wsum), B: round8(b / wsum)}
}

// chroma is the Lab chroma sqrt(a²+b²): distance from the neutral gray axis, i.e.
// colorfulness/saturation. Its difference between two images is the saturation
// shift the frontend reports.
func chroma(l color.Lab) float64 { return math.Hypot(l.A, l.B) }

// nearestMatches maps every reference swatch to its closest target swatch by
// ΔE00, so a reference color's fate in the target is explicit even when palette
// ordering differs.
func nearestMatches(ref, tgt color.Palette) []SwatchMatch {
	out := make([]SwatchMatch, 0, len(ref))
	for _, rs := range ref {
		best, bestD := tgt[0], color.Distance(rs.Lab(), tgt[0].Lab())
		for _, ts := range tgt[1:] {
			if d := color.Distance(rs.Lab(), ts.Lab()); d < bestD {
				best, bestD = ts, d
			}
		}
		out = append(out, SwatchMatch{Ref: rs, Target: best, DeltaE: bestD})
	}
	return out
}

// weightedMeanDeltaE averages the match distances weighted by reference-swatch
// coverage, so a small accent color drifting matters less than the dominant
// color drifting. Palette weights sum to ~1, so this is essentially Σ w·ΔE.
func weightedMeanDeltaE(matches []SwatchMatch) float64 {
	var sum, wsum float64
	for _, m := range matches {
		sum += m.Ref.Weight * m.DeltaE
		wsum += m.Ref.Weight
	}
	if wsum == 0 {
		return 0
	}
	return sum / wsum
}

// round8 rounds a [0,255] float to the nearest byte.
func round8(v float64) uint8 { return uint8(math.Round(v)) }
