package color_test

import (
	"encoding/json"
	"image"
	"math"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/color"
)

func TestParseHex(t *testing.T) {
	cases := []struct {
		in      string
		want    color.RGB
		wantErr bool
	}{
		{"#FF0000", color.RGB{R: 255}, false},
		{"00ff00", color.RGB{G: 255}, false},
		{"#00f", color.RGB{B: 255}, false},
		{"  #ABCDEF  ", color.RGB{R: 0xAB, G: 0xCD, B: 0xEF}, false},
		{"", color.RGB{}, true},
		{"#12345", color.RGB{}, true},
		{"#gg0000", color.RGB{}, true},
	}
	for _, c := range cases {
		got, err := color.ParseHex(c.in)
		if (err != nil) != c.wantErr {
			t.Fatalf("ParseHex(%q) err=%v, wantErr=%v", c.in, err, c.wantErr)
		}
		if !c.wantErr && got != c.want {
			t.Fatalf("ParseHex(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestHexRoundTrip(t *testing.T) {
	in := color.RGB{R: 18, G: 52, B: 86}
	got, err := color.ParseHex(in.Hex())
	if err != nil || got != in {
		t.Fatalf("round-trip %s = %+v, err=%v", in.Hex(), got, err)
	}
}

func TestLab(t *testing.T) {
	cases := []struct {
		rgb     color.RGB
		l, a, b float64
	}{
		{color.RGB{R: 255, G: 255, B: 255}, 100, 0, 0},
		{color.RGB{}, 0, 0, 0},
		{color.RGB{R: 255}, 53.2408, 80.0925, 67.2032},
		{color.RGB{G: 255}, 87.7347, -86.1827, 83.1793},
		{color.RGB{B: 255}, 32.2970, 79.1875, -107.8602},
	}
	for _, c := range cases {
		got := c.rgb.Lab()
		if !approx(got.L, c.l, 0.01) || !approx(got.A, c.a, 0.01) || !approx(got.B, c.b, 0.01) {
			t.Fatalf("%s Lab = %+v, want {%.4f %.4f %.4f}", c.rgb.Hex(), got, c.l, c.a, c.b)
		}
	}
}

// TestDistanceCIEDE2000 checks the implementation against reference pairs from
// Sharma, Wu & Dalal (2005), the canonical CIEDE2000 verification data.
func TestDistanceCIEDE2000(t *testing.T) {
	cases := []struct {
		a, b color.Lab
		want float64
	}{
		{color.Lab{L: 50, A: 2.6772, B: -79.7751}, color.Lab{L: 50, A: 0, B: -82.7485}, 2.0425},
		{color.Lab{L: 50, A: 0, B: 0}, color.Lab{L: 50, A: -1, B: 2}, 2.3669},
		{color.Lab{L: 60.2574, A: -34.0099, B: 36.2677}, color.Lab{L: 60.4626, A: -34.1751, B: 39.4387}, 1.2644},
		{color.Lab{L: 50, A: 2.49, B: -0.001}, color.Lab{L: 50, A: -2.49, B: 0.0009}, 7.1792},
		{color.Lab{L: 22.7233, A: 20.0904, B: -46.6940}, color.Lab{L: 23.0331, A: 14.9730, B: -42.5619}, 2.0373},
	}
	for _, c := range cases {
		if got := color.Distance(c.a, c.b); !approx(got, c.want, 1e-4) {
			t.Fatalf("Distance(%+v,%+v) = %.4f, want %.4f", c.a, c.b, got, c.want)
		}
	}
	// Distance is symmetric and zero for identical colors.
	x := color.Lab{L: 40, A: 10, B: -20}
	if d := color.Distance(x, x); d != 0 {
		t.Fatalf("Distance(x,x) = %v, want 0", d)
	}
}

func TestQuantize(t *testing.T) {
	// Solid image -> single dominant swatch covering all pixels.
	solid := fill(image.NewRGBA(image.Rect(0, 0, 8, 8)), color.RGB{R: 200, G: 100, B: 50})
	pal := color.Quantize(solid, 6)
	if len(pal) != 1 || pal[0].RGB != (color.RGB{R: 200, G: 100, B: 50}) || !approx(pal[0].Weight, 1.0, 1e-9) {
		t.Fatalf("solid palette = %+v", pal)
	}

	// Two-color image split down the middle -> two equal swatches, dominant first.
	two := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			c := color.RGB{R: 255}
			if x >= 5 {
				c = color.RGB{B: 255}
			}
			set(two, x, y, c)
		}
	}
	pal = color.Quantize(two, 6)
	if len(pal) != 2 {
		t.Fatalf("two-color palette len = %d, want 2: %+v", len(pal), pal)
	}
	for _, s := range pal {
		if !approx(s.Weight, 0.5, 1e-9) {
			t.Fatalf("swatch weight = %v, want 0.5", s.Weight)
		}
	}
}

func TestQuantizeEmpty(t *testing.T) {
	if pal := color.Quantize(image.NewRGBA(image.Rect(0, 0, 4, 4)), 4); pal != nil {
		t.Fatalf("fully transparent image palette = %+v, want nil", pal)
	}
}

func TestSwatchJSON(t *testing.T) {
	b, err := json.Marshal(color.Swatch{RGB: color.RGB{R: 0xAA, G: 0xBB, B: 0xCC}, Weight: 0.123456})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `{"hex":"#aabbcc","weight":0.1235}` {
		t.Fatalf("swatch JSON = %s", got)
	}
}

func approx(got, want, tol float64) bool { return math.Abs(got-want) <= tol }

func fill(img *image.RGBA, c color.RGB) *image.RGBA {
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	return img
}

// set writes one opaque pixel directly, avoiding an image/color import that would
// shadow this package's own color import name.
func set(img *image.RGBA, x, y int, c color.RGB) {
	i := img.PixOffset(x, y)
	img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
}
