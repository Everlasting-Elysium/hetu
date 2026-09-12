package imgcompare

// ToneKey is the overall brightness class of an image, decided from its mean
// luma against the shared shadowMax/highlightMin cutoffs (see luma.go) so it can
// never disagree with the lighting zones.
type ToneKey string

const (
	// KeyHigh is a bright image (mean luma >= highlightMin).
	KeyHigh ToneKey = "high"
	// KeyMid is a mid-brightness image.
	KeyMid ToneKey = "mid"
	// KeyLow is a dark image (mean luma < shadowMax).
	KeyLow ToneKey = "low"
)

// ToneStats are one image's brightness metrics plus the histogram the frontend
// draws. Histogram is normalized (sums to 1); mean/median/std/dynamicRange are
// in luma units [0,255]. StdDev doubles as the contrast indicator.
type ToneStats struct {
	Histogram    [HistogramBins]float64 `json:"histogram"`
	Mean         float64                `json:"mean"`
	Median       float64                `json:"median"`
	StdDev       float64                `json:"std_dev"`
	DynamicRange float64                `json:"dynamic_range"`
	Key          ToneKey                `json:"key"`
}

// ToneResult is the tone-dimension comparison: each image's stats plus the
// histogram intersection driving the score.
type ToneResult struct {
	Score            float64   `json:"score"`
	RefStats         ToneStats `json:"ref_stats"`
	TargetStats      ToneStats `json:"target_stats"`
	HistIntersection float64   `json:"hist_intersection"` // Σ min(refBin,tgtBin) in [0,1]
}

// keyFromMean classifies mean luma using the shared zone cutoffs.
func keyFromMean(mean float64) ToneKey {
	switch {
	case mean >= highlightMin:
		return KeyHigh
	case mean < shadowMax:
		return KeyLow
	default:
		return KeyMid
	}
}

// toneStatsFromField builds the full per-image tone stats from a luma field,
// computing the histogram once and reusing it for the scalar stats.
func toneStatsFromField(f lumaField) ToneStats {
	h := histogram(f)
	s := statsFromField(f, h)
	return ToneStats{
		Histogram:    h,
		Mean:         s.mean,
		Median:       s.median,
		StdDev:       s.stdDev,
		DynamicRange: s.dynamicRange,
		Key:          keyFromMean(s.mean),
	}
}

// histogramIntersection is Σ min(a[i],b[i]) over normalized histograms: 1 when
// the two brightness distributions are identical, 0 when disjoint (e.g. an
// all-black vs an all-white image). It is chosen over EMD because it is
// symmetric, bounded in [0,1], and maps straight to a 0-100 score with no free
// parameter, while still capturing mean and spread differences implicitly.
func histogramIntersection(a, b [HistogramBins]float64) float64 {
	var sum float64
	for i := range a {
		sum += min(a[i], b[i])
	}
	return sum
}

// toneResultFromFields compares two already-computed luma fields. The score is
// the histogram intersection scaled to 0-100.
func toneResultFromFields(ref, tgt lumaField) ToneResult {
	refStats := toneStatsFromField(ref)
	tgtStats := toneStatsFromField(tgt)
	inter := histogramIntersection(refStats.Histogram, tgtStats.Histogram)
	return ToneResult{
		Score:            100 * clamp01(inter),
		RefStats:         refStats,
		TargetStats:      tgtStats,
		HistIntersection: inter,
	}
}
