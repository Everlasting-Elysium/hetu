package imgcompare_test

import (
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/imgcompare"
)

// TestToneKeyAndMean pins the exact Rec.709 luma mean of solid fixtures (luma on
// gamma-encoded sRGB bytes, so red -> 0.2126*255 = 54.213, not a linearized
// value) and the resulting high/mid/low key against the shared 85/170 cutoffs.
func TestToneKeyAndMean(t *testing.T) {
	cases := []struct {
		name string
		c    color.RGB
		mean float64
		key  imgcompare.ToneKey
	}{
		{"black-low", color.RGB{}, 0, imgcompare.KeyLow},
		{"red-low", color.RGB{R: 255}, 0.2126 * 255, imgcompare.KeyLow},
		{"gray128-mid", color.RGB{R: 128, G: 128, B: 128}, 128, imgcompare.KeyMid},
		{"green-high", color.RGB{G: 255}, 0.7152 * 255, imgcompare.KeyHigh},
		{"white-high", color.RGB{R: 255, G: 255, B: 255}, 255, imgcompare.KeyHigh},
	}
	for _, c := range cases {
		img := solid(16, 16, c.c)
		st := imgcompare.Tone(img, img).RefStats
		if !approx(st.Mean, c.mean, 0.001) {
			t.Fatalf("%s: mean = %.4f, want %.4f", c.name, st.Mean, c.mean)
		}
		if !approx(st.Median, c.mean, 0.001) || !approx(st.StdDev, 0, 1e-9) {
			t.Fatalf("%s: median=%.4f std=%.4f, want %.4f / 0", c.name, st.Median, st.StdDev, c.mean)
		}
		if st.Key != c.key {
			t.Fatalf("%s: key = %q, want %q", c.name, st.Key, c.key)
		}
	}
}

// TestToneSplitHistogram pins the histogram, mean, std-dev, and dynamic range of
// a 16×16 image split red|blue down the middle: red luma 54.213 -> bin 6, blue
// luma 18.411 -> bin 2, 50/50, so mean=36.312, std=17.901, range=(6-2)*8=32.
func TestToneSplitHistogram(t *testing.T) {
	const (
		redLuma   = 0.2126 * 255 // 54.213
		blueLuma  = 0.0722 * 255 // 18.411
		wantMean  = (redLuma + blueLuma) / 2
		wantStd   = (redLuma - blueLuma) / 2 // ±equal deviation from the mean
		wantRange = float64(6-2) * (256.0 / imgcompare.HistogramBins)
	)
	img := vSplit(16, 16, color.RGB{R: 255}, color.RGB{B: 255})
	st := imgcompare.Tone(img, img).RefStats

	if !approx(st.Histogram[2], 0.5, 1e-9) || !approx(st.Histogram[6], 0.5, 1e-9) {
		t.Fatalf("histogram = %v, want bins[2]=bins[6]=0.5", st.Histogram)
	}
	var total float64
	for _, v := range st.Histogram {
		total += v
	}
	if !approx(total, 1, 1e-9) {
		t.Fatalf("histogram sum = %v, want 1", total)
	}
	if !approx(st.Mean, wantMean, 0.001) || !approx(st.Median, wantMean, 0.001) {
		t.Fatalf("mean=%.4f median=%.4f, want %.4f", st.Mean, st.Median, wantMean)
	}
	if !approx(st.StdDev, wantStd, 0.001) {
		t.Fatalf("std = %.4f, want %.4f", st.StdDev, wantStd)
	}
	if !approx(st.DynamicRange, wantRange, 1e-9) {
		t.Fatalf("dynamicRange = %.4f, want %.4f", st.DynamicRange, wantRange)
	}
	if st.Key != imgcompare.KeyLow { // mean 36.3 < 85
		t.Fatalf("key = %q, want low", st.Key)
	}
}

// TestToneBlackVsWhite: opposite extremes have disjoint histograms (bin 0 vs bin
// 31), so intersection and score are 0, and the keys are low vs high.
func TestToneBlackVsWhite(t *testing.T) {
	black := solid(32, 32, color.RGB{})
	white := solid(32, 32, color.RGB{R: 255, G: 255, B: 255})
	res := imgcompare.Tone(black, white)

	if !approx(res.RefStats.Histogram[0], 1, 1e-9) {
		t.Fatalf("black histogram[0] = %v, want 1", res.RefStats.Histogram[0])
	}
	if !approx(res.TargetStats.Histogram[imgcompare.HistogramBins-1], 1, 1e-9) {
		t.Fatalf("white histogram[last] = %v, want 1", res.TargetStats.Histogram[imgcompare.HistogramBins-1])
	}
	if res.RefStats.Key != imgcompare.KeyLow || res.TargetStats.Key != imgcompare.KeyHigh {
		t.Fatalf("keys = %q / %q, want low / high", res.RefStats.Key, res.TargetStats.Key)
	}
	if !approx(res.HistIntersection, 0, 1e-9) || !approx(res.Score, 0, 1e-9) {
		t.Fatalf("intersection=%v score=%v, want 0 / 0", res.HistIntersection, res.Score)
	}
}

func TestToneIdentical(t *testing.T) {
	img := solid(20, 20, color.RGB{R: 90, G: 110, B: 130})
	res := imgcompare.Tone(img, img)
	if !approx(res.Score, 100, 1e-9) || !approx(res.HistIntersection, 1, 1e-9) {
		t.Fatalf("Tone(identical) score=%v intersection=%v, want 100 / 1", res.Score, res.HistIntersection)
	}
}

// TestToneAndLightingConsistency proves the shared-luma-pass entry point returns
// exactly what the two standalone entry points do.
func TestToneAndLightingConsistency(t *testing.T) {
	a := vSplit(24, 24, color.RGB{R: 255}, color.RGB{R: 20, G: 20, B: 20})
	b := solid(24, 24, color.RGB{R: 128, G: 128, B: 128})
	tone, light := imgcompare.ToneAndLighting(a, b)
	if !approx(tone.Score, imgcompare.Tone(a, b).Score, 1e-9) {
		t.Fatalf("combined tone score %v != standalone %v", tone.Score, imgcompare.Tone(a, b).Score)
	}
	if !approx(light.Score, imgcompare.Lighting(a, b).Score, 1e-9) {
		t.Fatalf("combined lighting score %v != standalone %v", light.Score, imgcompare.Lighting(a, b).Score)
	}
}
