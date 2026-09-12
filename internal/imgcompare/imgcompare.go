// Package imgcompare is a pure-Go metric engine for "image comparison": scoring
// how closely a target image (a study/copy or a re-shot photo) reproduces a
// reference across five dimensions — color, tone, lighting, action, and element.
//
// The three pixel dimensions (color, tone, lighting; see color.go, tone.go,
// lighting.go) take two decoded image.Image values and compute everything from
// raw pixels: palette ΔE00 (delegated to internal/color, never reimplemented),
// Rec.709 luma histograms, and shadow/mid/highlight zones. The two semantic
// dimensions (action, element; see action.go, element.go) are pure functions
// over data the caller already fetched from the AI sidecar — WD-tagger tag maps
// and CLIP embeddings — so this package issues no network, disk, or database I/O
// and depends only on the standard library, github.com/disintegration/imaging,
// and internal/color. That keeps every dimension independently unit-testable
// against synthesized fixtures (see the _test.go files).
//
// Each dimension yields a 0-100 Score (100 = identical/very close, 0 = wholly
// different) plus frontend-drawable detail (histogram buckets, tri-zone splits,
// both palettes, ΔE00 values, tag diffs). Aggregate merges a chosen subset of
// dimension scores into an overall 0-100 total. The HTTP compare endpoint (a
// later PR) is the intended consumer: it decodes images, fetches tags/embeddings
// from the sidecar, calls the dimension functions here, and serializes the
// results — this package owns only the math.
package imgcompare

// Dimension identifies one comparison axis. It is the map key Aggregate averages
// over and the discriminator the HTTP layer uses to select which dimensions to
// run and report.
type Dimension string

const (
	// DimColor scores palette/ΔE00 similarity (see Color).
	DimColor Dimension = "color"
	// DimTone scores brightness-histogram similarity (see Tone).
	DimTone Dimension = "tone"
	// DimLighting scores shadow/highlight-zone similarity (see Lighting).
	DimLighting Dimension = "lighting"
	// DimAction scores pose-tag overlap (see Action).
	DimAction Dimension = "action"
	// DimElement scores subject-tag and embedding similarity (see Element).
	DimElement Dimension = "element"
)

// Aggregate averages the selected dimension scores into an overall 0-100 total
// with equal weight (1/N over the N entries present). The equal split is a
// deliberate placeholder: per-dimension weighting is left to the caller or a
// later PR once real usage data exists. Returns 0 for an empty selection.
func Aggregate(scores map[Dimension]float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	var sum float64
	for _, s := range scores {
		sum += s
	}
	return sum / float64(len(scores))
}

// clamp01 constrains x to [0,1]; similarity terms feed it before scaling to 100.
func clamp01(x float64) float64 {
	switch {
	case x < 0:
		return 0
	case x > 1:
		return 1
	default:
		return x
	}
}
