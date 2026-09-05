package image

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"

	goexif "github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

var _ kernel.MetadataExtractor = (*Handler)(nil)

// ExtractMetadata reads EXIF, IPTC, and XMP metadata from src. Failures in
// individual sections are swallowed: a JPEG with EXIF but no IPTC still
// returns whatever was found. Implements kernel.MetadataExtractor.
func (h *Handler) ExtractMetadata(_ context.Context, src io.ReadSeeker) (domain.ExtractedMetadata, error) {
	md := domain.ExtractedMetadata{Annotations: make(map[string]any)}

	extractEXIF(src, &md)

	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return md, fmt.Errorf("seek for segments: %w", err)
	}
	segments, _ := scanJPEGSegments(src)
	if segments.iptc != nil {
		extractIPTC(segments.iptc, &md)
	}
	if segments.xmp != nil {
		extractXMP(segments.xmp, &md)
	}
	return md, nil
}

// extractEXIF populates md with EXIF fields using rwcarlsen/goexif.
func extractEXIF(src io.ReadSeeker, md *domain.ExtractedMetadata) {
	x, err := goexif.Decode(src)
	if err != nil {
		return
	}
	setStringTag(x, goexif.Make, domain.KeyExifCameraMake, md)
	setStringTag(x, goexif.Model, domain.KeyExifCameraModel, md)
	setStringTag(x, goexif.LensModel, domain.KeyExifLensModel, md)

	if tag, err := x.Get(goexif.ISOSpeedRatings); err == nil {
		if v, err := tag.Int(0); err == nil && v > 0 {
			md.Annotations[domain.KeyExifISO] = v
		}
	}
	if tag, err := x.Get(goexif.FNumber); err == nil {
		if n, d, err := tag.Rat2(0); err == nil && d != 0 {
			f := float64(n) / float64(d)
			md.Annotations[domain.KeyExifFNumber] = fmt.Sprintf("f/%.1f", f)
		}
	}
	if tag, err := x.Get(goexif.ExposureTime); err == nil {
		md.Annotations[domain.KeyExifExposure] = formatExposure(tag)
	}
	if tag, err := x.Get(goexif.FocalLength); err == nil {
		if n, d, err := tag.Rat2(0); err == nil && d != 0 {
			fl := float64(n) / float64(d)
			md.Annotations[domain.KeyExifFocalLength] = fmt.Sprintf("%.0fmm", fl)
		}
	}
	if lat, lng, err := x.LatLong(); err == nil {
		md.Annotations[domain.KeyExifGPSLatitude] = roundGPS(lat)
		md.Annotations[domain.KeyExifGPSLongitude] = roundGPS(lng)
	}
	if dt, err := x.DateTime(); err == nil && !dt.IsZero() {
		md.DateTime = dt
		md.Annotations[domain.KeyExifDateTime] = dt.Format("2006-01-02T15:04:05Z07:00")
	}
}

func setStringTag(x *goexif.Exif, field goexif.FieldName, key string, md *domain.ExtractedMetadata) {
	tag, err := x.Get(field)
	if err != nil {
		return
	}
	s := strings.TrimSpace(tag.String())
	s = strings.Trim(s, `"`)
	if s != "" {
		md.Annotations[key] = s
	}
}

// formatExposure renders an ExposureTime EXIF rational as a human string.
func formatExposure(tag *tiff.Tag) string {
	n, d, err := tag.Rat2(0)
	if err != nil || d == 0 {
		return tag.String()
	}
	if n == 0 {
		return "0"
	}
	if n >= d {
		return fmt.Sprintf("%.1fs", float64(n)/float64(d))
	}
	// Express as 1/N for sub-second exposures.
	denom := float64(d) / float64(n)
	return fmt.Sprintf("1/%d", int(math.Round(denom)))
}

// roundGPS rounds a GPS coordinate to 6 decimal places (~0.11 m precision).
func roundGPS(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
