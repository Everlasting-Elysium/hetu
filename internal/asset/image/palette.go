package image

import (
	"context"
	"fmt"
	"io"

	"github.com/disintegration/imaging"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// sampleMaxDim and paletteSize reference the canonical palette defaults defined
// in the color package, so all asset handlers produce identical palettes.
const (
	sampleMaxDim = color.DefaultSampleMaxDim
	paletteSize  = color.DefaultPaletteSize
)

var _ kernel.PaletteExtractor = (*Handler)(nil)

// Palette decodes src, downsamples it, and returns up to paletteSize dominant
// colors ordered dominant-first. Implements kernel.PaletteExtractor.
func (h *Handler) Palette(_ context.Context, src io.ReadSeeker) (color.Palette, error) {
	img, err := imaging.Decode(src, imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return paletteFromImage(img), nil
}
