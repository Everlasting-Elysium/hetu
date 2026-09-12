package imgcompare_test

import (
	"image"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/imgcompare"
)

// TestLightingZonesBlackVsWhite: a black image is 100% shadow, a white image is
// 100% highlight, so the per-zone differences peak at 1.0.
func TestLightingZonesBlackVsWhite(t *testing.T) {
	black := solid(32, 32, color.RGB{})
	white := solid(32, 32, color.RGB{R: 255, G: 255, B: 255})
	res := imgcompare.Lighting(black, white)

	if !approx(res.RefZones.Shadow, 1, 1e-9) || !approx(res.RefZones.Highlight, 0, 1e-9) {
		t.Fatalf("black zones = %+v, want all shadow", res.RefZones)
	}
	if !approx(res.TargetZones.Highlight, 1, 1e-9) || !approx(res.TargetZones.Shadow, 0, 1e-9) {
		t.Fatalf("white zones = %+v, want all highlight", res.TargetZones)
	}
	if !approx(res.ZoneDiff.Shadow, 1, 1e-9) || !approx(res.ZoneDiff.Highlight, 1, 1e-9) {
		t.Fatalf("zone diff = %+v, want shadow=highlight=1", res.ZoneDiff)
	}
	// Both frames are internally flat (std-dev 0), so contrast is identical and
	// the score is the contrast term only: 0.75*0 + 0.25*1 = 0.25 -> 25.
	if !approx(res.RefContrast, 0, 1e-9) || !approx(res.TargetContrast, 0, 1e-9) {
		t.Fatalf("contrast = %v / %v, want 0 / 0", res.RefContrast, res.TargetContrast)
	}
	if !approx(res.Score, 25, 1e-9) {
		t.Fatalf("Score = %v, want 25 (zones disjoint, contrast equal)", res.Score)
	}
}

// TestLightingMidZones pins the tri-zone split of a three-band image: one third
// shadow (luma 40), one third midtone (luma 128), one third highlight (luma
// 200), each at exactly 1/3 coverage.
func TestLightingMidZones(t *testing.T) {
	img := image3Band(30, 30)
	res := imgcompare.Lighting(img, img)
	third := 1.0 / 3.0
	z := res.RefZones
	if !approx(z.Shadow, third, 1e-9) || !approx(z.Midtone, third, 1e-9) || !approx(z.Highlight, third, 1e-9) {
		t.Fatalf("zones = %+v, want each 1/3", z)
	}
	if !approx(res.Score, 100, 1e-9) { // identical image
		t.Fatalf("Score = %v, want 100", res.Score)
	}
}

func TestLightingIdentical(t *testing.T) {
	img := vSplit(20, 20, color.RGB{R: 30, G: 30, B: 30}, color.RGB{R: 220, G: 220, B: 220})
	res := imgcompare.Lighting(img, img)
	if !approx(res.Score, 100, 1e-9) || !res.LightMatches {
		t.Fatalf("Lighting(identical) score=%v matches=%v, want 100 / true", res.Score, res.LightMatches)
	}
}

// TestLightDirection: a bright top-left quadrant over a dark frame is classified
// top-left; a uniform frame is center-even.
func TestLightDirection(t *testing.T) {
	img := solid(32, 32, color.RGB{})
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			setPx(img, x, y, color.RGB{R: 255, G: 255, B: 255})
		}
	}
	if dir := imgcompare.Lighting(img, img).RefLight; dir != imgcompare.LightTopLeft {
		t.Fatalf("bright top-left quadrant -> %q, want top-left", dir)
	}

	flat := solid(16, 16, color.RGB{R: 120, G: 120, B: 120})
	if dir := imgcompare.Lighting(flat, flat).RefLight; dir != imgcompare.LightCenterEven {
		t.Fatalf("uniform frame -> %q, want center-even", dir)
	}
}

// image3Band returns a w×h image split into three equal horizontal bands whose
// gray levels fall in the shadow (40), midtone (128), and highlight (200) zones.
// Gray luma equals the byte value (Rec.709 weights sum to 1), and h is expected
// to be a multiple of 3 so the bands are exactly 1/3 each.
func image3Band(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bands := [3]color.RGB{
		{R: 40, G: 40, B: 40},    // shadow
		{R: 128, G: 128, B: 128}, // midtone
		{R: 200, G: 200, B: 200}, // highlight
	}
	for y := 0; y < h; y++ {
		c := bands[min(y/(h/3), 2)]
		for x := 0; x < w; x++ {
			setPx(img, x, y, c)
		}
	}
	return img
}
