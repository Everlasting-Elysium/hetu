package color

import (
	"image"

	"github.com/disintegration/imaging"
)

// DefaultSampleMaxDim caps the longest edge before quantization. Median-cut cost
// is linear in pixel count and dominant colors survive heavy downsampling, so
// 128 px keeps extraction fast even on large images.
const DefaultSampleMaxDim = 128

// DefaultPaletteSize is the number of dominant colors extracted per asset.
const DefaultPaletteSize = 6

// ExtractPalette downsamples img to fit maxDim and returns up to size dominant
// colors ordered dominant-first. It is the decode-agnostic tail shared by all
// asset handlers that implement kernel.PaletteExtractor: images decode directly,
// video decodes an ffmpeg keyframe, 3D decodes a Blender render, and all three
// converge here so palette results stay identical across asset kinds.
func ExtractPalette(img image.Image, maxDim, size int) Palette {
	small := imaging.Fit(img, maxDim, maxDim, imaging.Box)
	return Quantize(small, size)
}
