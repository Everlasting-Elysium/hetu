package color

import (
	"image"
	"sort"
)

// alphaThreshold skips near-transparent pixels so cut-outs are quantized by
// their visible colors, not the transparent background.
const alphaThreshold = 0x8000 // half of the 16-bit alpha range

// Quantize extracts up to k dominant colors from img using median-cut, returning
// a Palette ordered by descending weight (element 0 is the dominant color).
// Callers should downsample large images first; median-cut cost is linear in the
// pixel count. Returns nil when the image has no opaque pixels.
func Quantize(img image.Image, k int) Palette {
	pixels := collect(img)
	if len(pixels) == 0 || k < 1 {
		return nil
	}
	boxes := []box{newBox(pixels)}
	for len(boxes) < k {
		i := widest(boxes)
		if i < 0 {
			break // no box can be split further (each holds one color)
		}
		lo, hi := boxes[i].split()
		boxes = append(append(boxes[:i:i], lo, hi), boxes[i+1:]...)
	}
	total := float64(len(pixels))
	pal := make(Palette, 0, len(boxes))
	for _, b := range boxes {
		pal = append(pal, Swatch{RGB: b.average(), Weight: float64(len(b.pixels)) / total})
	}
	sort.Slice(pal, func(i, j int) bool { return pal[i].Weight > pal[j].Weight })
	return pal
}

// collect flattens opaque pixels into a slice of RGB triples.
func collect(img image.Image) []RGB {
	b := img.Bounds()
	out := make([]RGB, 0, b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA() // 16-bit premultiplied
			if a < alphaThreshold {
				continue
			}
			out = append(out, RGB{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8)})
		}
	}
	return out
}

// box is a contiguous run of pixels sharing a region of RGB space.
type box struct {
	pixels   []RGB
	min, max RGB
}

func newBox(pixels []RGB) box {
	b := box{pixels: pixels}
	b.shrink()
	return b
}

// shrink recomputes the tight RGB bounding box of the contained pixels.
func (b *box) shrink() {
	b.min = RGB{255, 255, 255}
	b.max = RGB{0, 0, 0}
	for _, p := range b.pixels {
		b.min.R, b.max.R = minU8(b.min.R, p.R), maxU8(b.max.R, p.R)
		b.min.G, b.max.G = minU8(b.min.G, p.G), maxU8(b.max.G, p.G)
		b.min.B, b.max.B = minU8(b.min.B, p.B), maxU8(b.max.B, p.B)
	}
}

// longestAxis returns the channel (0=R,1=G,2=B) with the widest spread.
func (b box) longestAxis() int {
	dr, dg, db := b.max.R-b.min.R, b.max.G-b.min.G, b.max.B-b.min.B
	if dr >= dg && dr >= db {
		return 0
	}
	if dg >= db {
		return 1
	}
	return 2
}

// split sorts the box along its longest axis and cuts it at the median, so each
// half holds roughly equal pixel counts (median-cut).
func (b box) split() (box, box) {
	axis := b.longestAxis()
	sort.Slice(b.pixels, func(i, j int) bool {
		return channel(b.pixels[i], axis) < channel(b.pixels[j], axis)
	})
	mid := len(b.pixels) / 2
	return newBox(b.pixels[:mid]), newBox(b.pixels[mid:])
}

// average is the mean color of the box, used as its representative swatch.
func (b box) average() RGB {
	var sr, sg, sb int
	for _, p := range b.pixels {
		sr, sg, sb = sr+int(p.R), sg+int(p.G), sb+int(p.B)
	}
	n := len(b.pixels)
	return RGB{R: uint8(sr / n), G: uint8(sg / n), B: uint8(sb / n)}
}

// widest returns the index of the splittable box with the largest axis spread,
// or -1 when every box holds a single color.
func widest(boxes []box) int {
	best, bestSpread := -1, 0
	for i, b := range boxes {
		if len(b.pixels) < 2 {
			continue
		}
		if s := b.spread(); s > bestSpread {
			best, bestSpread = i, s
		}
	}
	return best
}

func (b box) spread() int {
	dr, dg, db := int(b.max.R-b.min.R), int(b.max.G-b.min.G), int(b.max.B-b.min.B)
	return maxInt(dr, maxInt(dg, db))
}

func channel(c RGB, axis int) uint8 {
	switch axis {
	case 0:
		return c.R
	case 1:
		return c.G
	default:
		return c.B
	}
}

func minU8(a, b uint8) uint8 {
	if a < b {
		return a
	}
	return b
}

func maxU8(a, b uint8) uint8 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
