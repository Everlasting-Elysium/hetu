package imgcompare

import (
	"image"
	"math"
	"sort"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/disintegration/imaging"
)

// HistogramBins is the bucket count of every luma histogram. 32 buckets (8 luma
// values each) is fine enough to show tonal shape for the frontend yet coarse
// enough that downsampling noise does not fragment the distribution.
const HistogramBins = 32

// binWidth is the luma span covered by one histogram bucket (256/HistogramBins).
const binWidth = 256.0 / HistogramBins

// shadowMax and highlightMin are the luma cutoffs shared by the tone key (see
// tone.go) and the lighting zones (see lighting.go): luma < shadowMax is
// shadow/low-key, luma >= highlightMin is highlight/high-key, and the span in
// between is midtone/mid-key. One pair of numbers drives both dimensions so they
// can never disagree.
const (
	shadowMax    = 85.0
	highlightMin = 170.0
)

// Rec.709 luma weights. Luma (Y') is a weighted sum of the *gamma-encoded* sRGB
// byte values, NOT linearized luminance (Y) — this is the standard video "luma"
// used for tonal analysis, and the tests pin exact values (pure red ->
// 0.2126*255 = 54.213) to prove no accidental linearization crept in.
const (
	lumaR = 0.2126
	lumaG = 0.7152
	lumaB = 0.0722
)

// lumaField is one image reduced to its per-pixel Rec.709 luma in [0,255],
// computed once from a downsampled copy and shared by the tone and lighting
// dimensions so the luma pass never runs twice for the same image.
type lumaField struct {
	luma   []float64 // row-major, one entry per downsampled pixel
	width  int
	height int
}

// newLumaField downsamples img to at most color.DefaultSampleMaxDim on its long
// edge (reusing the color package's sampling cap for one consistent, fast pass)
// and computes per-pixel luma. imaging.Fit only shrinks, so small fixtures keep
// their exact pixels and thus their exact luma.
func newLumaField(img image.Image) lumaField {
	small := imaging.Fit(img, color.DefaultSampleMaxDim, color.DefaultSampleMaxDim, imaging.Box)
	b := small.Bounds()
	w, h := b.Dx(), b.Dy()
	f := lumaField{luma: make([]float64, 0, w*h), width: w, height: h}
	// small is *image.NRGBA: Pix holds non-premultiplied R,G,B,A bytes, so the
	// channel values are the true sRGB bytes we weight directly.
	for i := 0; i+3 < len(small.Pix); i += 4 {
		r, g, bl := float64(small.Pix[i]), float64(small.Pix[i+1]), float64(small.Pix[i+2])
		f.luma = append(f.luma, lumaR*r+lumaG*g+lumaB*bl)
	}
	return f
}

// bin maps a luma value to its histogram bucket index, clamped to the last bin.
func bin(luma float64) int {
	i := int(luma / binWidth)
	switch {
	case i >= HistogramBins:
		return HistogramBins - 1
	case i < 0:
		return 0
	default:
		return i
	}
}

// histogram returns the field's luma distribution normalized to sum to 1 (all
// zero for an empty field), ready both for the frontend and for intersecting
// against another image's histogram.
func histogram(f lumaField) [HistogramBins]float64 {
	var h [HistogramBins]float64
	if len(f.luma) == 0 {
		return h
	}
	for _, v := range f.luma {
		h[bin(v)]++
	}
	n := float64(len(f.luma))
	for i := range h {
		h[i] /= n
	}
	return h
}

// fieldStats holds the scalar tonal metrics derived from one luma field.
type fieldStats struct {
	mean, median, stdDev, dynamicRange float64
}

// statsFromField computes mean, median, population standard deviation (the
// contrast indicator), and dynamic range from f and its histogram h. Dynamic
// range is the luma span between the lowest and highest occupied buckets, so a
// flat solid image reports 0 and a full black-to-white image reports ~248.
func statsFromField(f lumaField, h [HistogramBins]float64) fieldStats {
	if len(f.luma) == 0 {
		return fieldStats{}
	}
	mean, std := meanStdDev(f)
	return fieldStats{mean: mean, median: median(f.luma), stdDev: std, dynamicRange: dynamicRange(h)}
}

// meanStdDev returns the mean and population standard deviation of the field's
// luma. Std-dev is the single contrast indicator shared by the tone stats and
// the lighting dimension, so "contrast" means exactly one number everywhere.
func meanStdDev(f lumaField) (float64, float64) {
	if len(f.luma) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range f.luma {
		sum += v
	}
	mean := sum / float64(len(f.luma))
	var sq float64
	for _, v := range f.luma {
		d := v - mean
		sq += d * d
	}
	return mean, math.Sqrt(sq / float64(len(f.luma)))
}

// median returns the middle luma value from a sorted copy of the samples, so the
// caller's field ordering is untouched.
func median(luma []float64) float64 {
	s := append([]float64(nil), luma...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// dynamicRange is the luma distance between the lowest and highest non-empty
// buckets, in luma units (bucket-index difference * binWidth).
func dynamicRange(h [HistogramBins]float64) float64 {
	lo, hi := -1, -1
	for i, v := range h {
		if v > 0 {
			if lo < 0 {
				lo = i
			}
			hi = i
		}
	}
	if lo < 0 {
		return 0
	}
	return float64(hi-lo) * binWidth
}
