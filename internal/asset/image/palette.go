package image

import (
	"context"
	"fmt"
	"io"

	"github.com/disintegration/imaging"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// sampleMaxDim caps the longest edge before quantization. Median-cut cost is
// linear in pixel count and dominant colors survive heavy downsampling, so this
// keeps palette extraction fast even on large images.
const sampleMaxDim = 128

// paletteSize is the number of dominant colors extracted per image.
const paletteSize = 6

var _ kernel.PaletteExtractor = (*Handler)(nil)

// Palette decodes src, downsamples it, and returns up to paletteSize dominant
// colors ordered dominant-first. Implements kernel.PaletteExtractor.
func (h *Handler) Palette(_ context.Context, src io.ReadSeeker) (color.Palette, error) {
	img, err := imaging.Decode(src, imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	small := imaging.Fit(img, sampleMaxDim, sampleMaxDim, imaging.Box)
	return color.Quantize(small, paletteSize), nil
}
