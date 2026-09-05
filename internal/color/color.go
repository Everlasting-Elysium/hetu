// Package color provides pure-Go color analysis for asset indexing: sRGB<->hex
// parsing, CIE-Lab conversion, perceptual distance (CIEDE2000, see distance.go),
// and median-cut palette extraction (see quantize.go). It has no I/O and depends
// only on the standard library so it can be unit-tested in isolation and reused
// by the image handler, the store's color index, and the DAM search endpoint.
package color

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// RGB is an 8-bit-per-channel sRGB color.
type RGB struct{ R, G, B uint8 }

// Lab is a CIE-Lab color (D65 white point). Perceptual distance between two Lab
// colors is computed with CIEDE2000 rather than RGB euclidean distance, which is
// too strict for color search (a lesson from Billfish's over-tight tolerance).
type Lab struct{ L, A, B float64 }

// Swatch is one palette color together with the fraction of image pixels it
// represents (0..1). A Palette is ordered dominant-first (highest weight).
type Swatch struct {
	RGB
	Weight float64
}

// Palette is an image's extracted colors, ordered by descending weight.
type Palette []Swatch

// ParseHex parses "#RGB", "#RRGGBB" (and the same without the leading '#') into
// an RGB. Any other form is rejected so callers can return a 400 on bad input.
func ParseHex(s string) (RGB, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	switch len(h) {
	case 3:
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	case 6:
	default:
		return RGB{}, fmt.Errorf("invalid hex color %q: want #RGB or #RRGGBB", s)
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(strings.ToLower(h), "%02x%02x%02x", &r, &g, &b); err != nil {
		return RGB{}, fmt.Errorf("invalid hex color %q: %w", s, err)
	}
	return RGB{R: r, G: g, B: b}, nil
}

// Hex renders the color as a lowercase "#rrggbb" string.
func (c RGB) Hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// Lab converts the sRGB color to CIE-Lab (D65) via linear-RGB and XYZ.
func (c RGB) Lab() Lab {
	r, g, b := linear(c.R), linear(c.G), linear(c.B)
	// Linear sRGB -> XYZ (D65 primaries), then normalize by the D65 white point.
	x := (0.4124564*r + 0.3575761*g + 0.1804375*b) / 0.95047
	y := 0.2126729*r + 0.7151522*g + 0.0721750*b
	z := (0.0193339*r + 0.1191920*g + 0.9503041*b) / 1.08883
	fx, fy, fz := labF(x), labF(y), labF(z)
	return Lab{L: 116*fy - 16, A: 500 * (fx - fy), B: 200 * (fy - fz)}
}

// linear inverts the sRGB gamma curve for one channel, returning [0,1].
func linear(v uint8) float64 {
	c := float64(v) / 255.0
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// labF is the CIE-Lab nonlinearity with the linear segment near black.
func labF(t float64) float64 {
	const d = 6.0 / 29.0
	if t > d*d*d {
		return math.Cbrt(t)
	}
	return t/(3*d*d) + 4.0/29.0
}

// MarshalJSON renders a swatch as {"hex":"#rrggbb","weight":0.42} so the palette
// stored in annotations and returned by the API shares one canonical shape.
func (s Swatch) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Hex    string  `json:"hex"`
		Weight float64 `json:"weight"`
	}{s.Hex(), math.Round(s.Weight*10000) / 10000})
}
