package image

import (
	"context"
	"io"

	goexif "github.com/rwcarlsen/goexif/exif"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// ExtractMetadata reads embedded EXIF (RAW and HEIC are EXIF/TIFF-based) plus the
// color space (ICC profile / EXIF ColorSpace) as extracted-layer annotations. It
// reuses the same readers as the pure-Go handler and never fails the scan; a
// format without embedded metadata (e.g. most EXR) simply yields nothing.
func (h *ProHandler) ExtractMetadata(_ context.Context, src io.ReadSeeker) (domain.ExtractedMetadata, error) {
	md := domain.ExtractedMetadata{Annotations: make(map[string]any)}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return md, nil
	}
	extractEXIF(src, &md)
	extractColorSpace(src, &md)
	return md, nil
}

// exifDimensions reads pixel dimensions from EXIF (bounded by exifScanCap, so no
// full-file decode). TIFF-based RAW embed PixelXDimension or ImageWidth/Length;
// formats without early EXIF (e.g. EXR, HEIC) return zero, which the indexer
// records as unknown dimensions. It avoids the expensive external decode, which
// happens once in Thumbnail.
func exifDimensions(src io.ReadSeeker) (int, int) {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return 0, 0
	}
	x, err := goexif.Decode(io.LimitReader(src, exifScanCap))
	if err != nil {
		return 0, 0
	}
	w := exifInt(x, goexif.PixelXDimension)
	ht := exifInt(x, goexif.PixelYDimension)
	if w == 0 || ht == 0 {
		w = exifInt(x, goexif.ImageWidth)
		ht = exifInt(x, goexif.ImageLength)
	}
	return w, ht
}

func exifInt(x *goexif.Exif, field goexif.FieldName) int {
	tag, err := x.Get(field)
	if err != nil {
		return 0
	}
	v, err := tag.Int(0)
	if err != nil || v < 0 {
		return 0
	}
	return v
}
