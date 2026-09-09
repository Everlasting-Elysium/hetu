// Package thumb encodes an already-decoded image into a JPEG thumbnail. It is
// the shared fit-and-encode tail that every raster-producing asset handler
// converges on (image, psd, font) so they all emit identical thumbnails from a
// decoded image, without duplicating the imaging calls or cross-importing an
// unexported helper.
package thumb

import (
	"fmt"
	stdimage "image"
	"io"

	"github.com/disintegration/imaging"
)

// Encode fits img into a maxDim×maxDim box (preserving aspect ratio, Lanczos
// resampling) and writes it to w as JPEG.
func Encode(img stdimage.Image, w io.Writer, maxDim int) error {
	fitted := imaging.Fit(img, maxDim, maxDim, imaging.Lanczos)
	if err := imaging.Encode(w, fitted, imaging.JPEG); err != nil {
		return fmt.Errorf("encode thumbnail: %w", err)
	}
	return nil
}

const (
	// MaxDim bounds a source image's declared width/height (px), and MaxPixels
	// bounds its total pixel count, so a malformed header claiming enormous
	// dimensions cannot trigger a giant allocation. Callers check the declared
	// size (via a header-only DecodeConfig) BEFORE decoding pixels.
	MaxDim    = 20000
	MaxPixels = 200_000_000
)

// WithinLimits reports whether an image of w×h pixels is safe to decode: both
// edges positive and within MaxDim, and the total within MaxPixels. The product
// is computed in int64 so a hostile w*h cannot overflow the check.
func WithinLimits(w, h int) bool {
	if w <= 0 || h <= 0 || w > MaxDim || h > MaxDim {
		return false
	}
	return int64(w)*int64(h) <= MaxPixels
}
