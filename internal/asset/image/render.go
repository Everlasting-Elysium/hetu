package image

import (
	"fmt"
	stdimage "image"
	"io"

	"github.com/disintegration/imaging"

	"github.com/Everlasting-Elysium/hetu/internal/color"
)

// encodeThumbnail fits img into a thumbMaxDim box (Lanczos) and writes it to w
// as JPEG. Shared by the pure-Go Handler and the external-tool ProHandler so
// both produce identical thumbnails once the source is decoded to an image.
func encodeThumbnail(img stdimage.Image, w io.Writer) error {
	thumb := imaging.Fit(img, thumbMaxDim, thumbMaxDim, imaging.Lanczos)
	if err := imaging.Encode(w, thumb, imaging.JPEG); err != nil {
		return fmt.Errorf("encode thumbnail: %w", err)
	}
	return nil
}

// paletteFromImage downsamples img and returns up to paletteSize dominant colors
// ordered dominant-first. Shared decode-agnostic tail of palette extraction.
func paletteFromImage(img stdimage.Image) color.Palette {
	small := imaging.Fit(img, sampleMaxDim, sampleMaxDim, imaging.Box)
	return color.Quantize(small, paletteSize)
}
