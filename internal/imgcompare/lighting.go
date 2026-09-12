package imgcompare

import "math"

// LightDir is a coarse heuristic light-source direction: which quarter of the
// frame is brightest, or center-even when the frame is roughly uniform. This is
// an estimate from brightness distribution only; precise light-direction
// judgment is left to a VLM.
type LightDir string

const (
	LightTopLeft     LightDir = "top-left"
	LightTopRight    LightDir = "top-right"
	LightBottomLeft  LightDir = "bottom-left"
	LightBottomRight LightDir = "bottom-right"
	LightCenterEven  LightDir = "center-even"
)

// lightEvenThreshold is the relative brightest-to-darkest quadrant spread below
// which the frame is called center-even. 0.10 means a <10% swing across
// quadrants counts as uniform lighting.
const lightEvenThreshold = 0.10

// Lighting score blend: zone distribution dominates, contrast similarity is a
// secondary term. These are reasonable placeholder weights, tunable later.
const (
	lightingZoneWeight     = 0.75
	lightingContrastWeight = 0.25
	contrastScale          = 128.0 // std-dev normalizer; ~max std of a 50/50 black-white split
)

// ToneZones are the shadow/midtone/highlight pixel fractions, each in [0,1] and
// summing to ~1, split at the shared shadowMax/highlightMin cutoffs (see
// luma.go): shadow luma<85, midtone 85..170, highlight luma>=170.
type ToneZones struct {
	Shadow    float64 `json:"shadow"`
	Midtone   float64 `json:"midtone"`
	Highlight float64 `json:"highlight"`
}

// LightingResult is the light/shadow comparison: each image's zones and coarse
// light direction, their differences, and the contrast (identical to the tone
// dimension's std-dev — the same number, reused).
type LightingResult struct {
	Score          float64   `json:"score"`
	RefZones       ToneZones `json:"ref_zones"`
	TargetZones    ToneZones `json:"target_zones"`
	ZoneDiff       ToneZones `json:"zone_diff"` // |target - ref| per zone
	RefContrast    float64   `json:"ref_contrast"`
	TargetContrast float64   `json:"target_contrast"`
	RefLight       LightDir  `json:"ref_light"`
	TargetLight    LightDir  `json:"target_light"`
	LightMatches   bool      `json:"light_matches"`
}

// zonesFromField counts the shared per-pixel luma array (already computed by the
// tone pass — never recomputed here) into the three tonal zones.
func zonesFromField(f lumaField) ToneZones {
	if len(f.luma) == 0 {
		return ToneZones{}
	}
	var sh, mid, hi float64
	for _, v := range f.luma {
		switch {
		case v < shadowMax:
			sh++
		case v >= highlightMin:
			hi++
		default:
			mid++
		}
	}
	n := float64(len(f.luma))
	return ToneZones{Shadow: sh / n, Midtone: mid / n, Highlight: hi / n}
}

// lightDirection splits the frame into a 2x2 grid, averages luma per quadrant,
// and names the brightest quadrant — or center-even when the spread is small.
// Heuristic only; see LightDir.
func lightDirection(f lumaField) LightDir {
	if f.width < 2 || f.height < 2 || len(f.luma) == 0 {
		return LightCenterEven
	}
	midX, midY := f.width/2, f.height/2
	var sum, cnt [4]float64
	for i, v := range f.luma {
		q := 0
		if i%f.width >= midX {
			q++ // right half
		}
		if i/f.width >= midY {
			q += 2 // bottom half
		}
		sum[q] += v
		cnt[q]++
	}
	var avg [4]float64
	var overall float64
	for q := range avg {
		if cnt[q] > 0 {
			avg[q] = sum[q] / cnt[q]
		}
		overall += avg[q]
	}
	overall /= 4
	hiQ, hiV, loV := 0, avg[0], avg[0]
	for q, v := range avg {
		if v > hiV {
			hiQ, hiV = q, v
		}
		if v < loV {
			loV = v
		}
	}
	if overall == 0 || (hiV-loV)/overall < lightEvenThreshold {
		return LightCenterEven
	}
	return [4]LightDir{LightTopLeft, LightTopRight, LightBottomLeft, LightBottomRight}[hiQ]
}

// lightingResultFromFields compares two already-computed luma fields. The score
// blends tri-zone overlap (Σ min per zone) with contrast similarity; light
// direction is reported as detail (and LightMatches) but does not drive the
// score, since the direction estimate is deliberately coarse.
func lightingResultFromFields(ref, tgt lumaField) LightingResult {
	refZones, tgtZones := zonesFromField(ref), zonesFromField(tgt)
	_, refStd := meanStdDev(ref)
	_, tgtStd := meanStdDev(tgt)
	refLight, tgtLight := lightDirection(ref), lightDirection(tgt)

	overlap := min(refZones.Shadow, tgtZones.Shadow) +
		min(refZones.Midtone, tgtZones.Midtone) +
		min(refZones.Highlight, tgtZones.Highlight)
	contrastSim := 1 - math.Min(math.Abs(refStd-tgtStd)/contrastScale, 1)
	score := 100 * clamp01(lightingZoneWeight*overlap+lightingContrastWeight*contrastSim)

	return LightingResult{
		Score:       score,
		RefZones:    refZones,
		TargetZones: tgtZones,
		ZoneDiff: ToneZones{
			Shadow:    math.Abs(tgtZones.Shadow - refZones.Shadow),
			Midtone:   math.Abs(tgtZones.Midtone - refZones.Midtone),
			Highlight: math.Abs(tgtZones.Highlight - refZones.Highlight),
		},
		RefContrast:    refStd,
		TargetContrast: tgtStd,
		RefLight:       refLight,
		TargetLight:    tgtLight,
		LightMatches:   refLight == tgtLight,
	}
}
