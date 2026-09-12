package imgcompare_test

import (
	"image"
	"math"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/imgcompare"
)

// approx reports whether got is within tol of want (float comparison helper,
// mirroring internal/color's test style).
func approx(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

// solid returns a w×h opaque image filled with one color.
func solid(w, h int, c color.RGB) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	return img
}

// setPx writes one opaque pixel directly, avoiding an image/color import that
// would shadow the color package name.
func setPx(img *image.RGBA, x, y int, c color.RGB) {
	i := img.PixOffset(x, y)
	img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
}

// vSplit returns a w×h image whose left half is left and right half is right,
// split at x == w/2.
func vSplit(w, h int, left, right color.RGB) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := left
			if x >= w/2 {
				c = right
			}
			setPx(img, x, y, c)
		}
	}
	return img
}

// sameStrings compares two string slices element-wise, treating nil and empty
// as equal so diff results (which are nil when empty) compare cleanly.
func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestAggregate(t *testing.T) {
	cases := []struct {
		name   string
		scores map[imgcompare.Dimension]float64
		want   float64
	}{
		{"empty", map[imgcompare.Dimension]float64{}, 0},
		{"single", map[imgcompare.Dimension]float64{imgcompare.DimColor: 80}, 80},
		{"two-equal-weight", map[imgcompare.Dimension]float64{
			imgcompare.DimColor: 80, imgcompare.DimTone: 40,
		}, 60},
		{"all-five", map[imgcompare.Dimension]float64{
			imgcompare.DimColor: 100, imgcompare.DimTone: 50, imgcompare.DimLighting: 0,
			imgcompare.DimAction: 100, imgcompare.DimElement: 50,
		}, 60},
	}
	for _, c := range cases {
		if got := imgcompare.Aggregate(c.scores); !approx(got, c.want, 1e-9) {
			t.Fatalf("Aggregate(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}
