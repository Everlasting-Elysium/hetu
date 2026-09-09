package image

import (
	stdimage "image"
	"io"

	"github.com/Everlasting-Elysium/hetu/internal/asset/thumb"
	"github.com/Everlasting-Elysium/hetu/internal/color"
)

// encodeThumbnail fits img into a thumbMaxDim box (Lanczos) and writes it to w
// as JPEG via the shared thumb encoder, so the pure-Go Handler, the external-
// tool ProHandler, and the psd/font handlers all emit identical thumbnails once
// the source is decoded to an image.
func encodeThumbnail(img stdimage.Image, w io.Writer) error {
	return thumb.Encode(img, w, thumbMaxDim)
}

// paletteFromImage downsamples img and returns up to paletteSize dominant colors
// ordered dominant-first. It delegates to color.ExtractPalette, the shared tail
// every palette-extracting handler (image, video, 3D) converges on.
func paletteFromImage(img stdimage.Image) color.Palette {
	return color.ExtractPalette(img, sampleMaxDim, paletteSize)
}
