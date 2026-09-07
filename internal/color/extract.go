package color

import (
	"fmt"
	"image"
	"io"

	"github.com/disintegration/imaging"
)

// DefaultSampleMaxDim caps the longest edge before quantization. Median-cut cost
// is linear in pixel count and dominant colors survive heavy downsampling, so
// 128 px keeps extraction fast even on large images.
const DefaultSampleMaxDim = 128

// DefaultPaletteSize is the number of dominant colors extracted per asset.
const DefaultPaletteSize = 6

// ExtractPalette downsamples img to fit maxDim and returns up to size dominant
// colors ordered dominant-first. It is the decode-agnostic tail every palette
// source converges on, so results stay identical across asset kinds.
func ExtractPalette(img image.Image, maxDim, size int) Palette {
	small := imaging.Fit(img, maxDim, maxDim, imaging.Box)
	return Quantize(small, size)
}

// ExtractPaletteFromReader decodes an encoded image from r and returns its
// default palette. It is the shared entry point for palette sources whose pixels
// live in an already-rendered raster — a video/3D thumbnail (ffmpeg keyframe,
// client screenshot, or an optional Blender render) — so neither the indexer nor
// the thumbnail-upload handler re-decodes or re-renders on its own.
func ExtractPaletteFromReader(r io.Reader) (Palette, error) {
	img, err := imaging.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return ExtractPalette(img, DefaultSampleMaxDim, DefaultPaletteSize), nil
}
