package image

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"

	goexif "github.com/rwcarlsen/goexif/exif"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// pngSignature is the 8-byte PNG magic header.
const pngSignature = "\x89PNG\r\n\x1a\n"

// maxICCSize caps an inflated ICC profile to guard against decompression bombs.
const maxICCSize = 4 << 20

// extractColorSpace records the asset's embedded color space as extracted-layer
// annotations: the ICC profile description (color.icc_profile) and a normalized
// space name (color.space). It never fails — an absent or unreadable profile
// simply yields no annotations — so callers can invoke it unconditionally.
func extractColorSpace(src io.ReadSeeker, md *domain.ExtractedMetadata) {
	var space, desc string
	if icc := locateICC(src); len(icc) > 0 {
		if _, desc = parseICCProfile(icc); desc != "" {
			md.Annotations[domain.KeyColorICCProfile] = desc
			space = normalizeColorSpace(desc)
		}
	}
	if space == "" {
		space = exifColorSpace(src)
	}
	if space != "" {
		md.Annotations[domain.KeyColorSpace] = space
	}
}

// locateICC returns the raw ICC profile embedded in src (JPEG APP2 or PNG iCCP),
// or nil. It sniffs the container by magic bytes and rewinds src before reading.
func locateICC(src io.ReadSeeker) []byte {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	var magic [8]byte
	n, _ := io.ReadFull(src, magic[:])
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	switch {
	case n >= 2 && magic[0] == 0xFF && magic[1] == 0xD8:
		seg, _ := scanJPEGSegments(src)
		return seg.icc
	case n >= 8 && string(magic[:8]) == pngSignature:
		return pngICCProfile(src)
	}
	return nil
}

// pngICCProfile scans PNG chunks for an iCCP profile and returns the inflated
// ICC bytes, or nil. The iCCP payload is: name(1-79)\0 + method(1) + zlib data.
// Non-iCCP chunks are streamed past without allocating, and the iCCP buffer is
// capped, so a crafted chunk length can never trigger a huge allocation.
func pngICCProfile(src io.Reader) []byte {
	var sig [8]byte
	if _, err := io.ReadFull(src, sig[:]); err != nil || string(sig[:]) != pngSignature {
		return nil
	}
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(src, hdr[:]); err != nil {
			return nil
		}
		length := int64(binary.BigEndian.Uint32(hdr[0:4]))
		ctype := string(hdr[4:8])
		if ctype == "IDAT" || ctype == "IEND" {
			return nil // an iCCP profile, if present, precedes the pixel data
		}
		if ctype != "iCCP" {
			// Skip data + CRC without trusting length to size an allocation.
			if _, err := io.CopyN(io.Discard, src, length+4); err != nil {
				return nil
			}
			continue
		}
		if length <= 0 || length > maxICCSize {
			return nil
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(src, data); err != nil {
			return nil
		}
		if _, err := io.CopyN(io.Discard, src, 4); err != nil { // skip CRC
			return nil
		}
		return inflateICCP(data)
	}
}

// inflateICCP strips the iCCP name + compression byte and zlib-inflates the rest.
func inflateICCP(data []byte) []byte {
	i := bytes.IndexByte(data, 0)
	if i < 0 || i+2 > len(data) {
		return nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(data[i+2:]))
	if err != nil {
		return nil
	}
	defer func() { _ = zr.Close() }()
	out, err := io.ReadAll(io.LimitReader(zr, maxICCSize))
	if err != nil {
		return nil
	}
	return out
}

// exifColorSpace reads the EXIF ColorSpace tag (0xA001): 1 = sRGB, 0xFFFF =
// Uncalibrated (commonly a wide-gamut profile). Returns "" when absent.
func exifColorSpace(src io.ReadSeeker) string {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	x, err := goexif.Decode(io.LimitReader(src, exifScanCap))
	if err != nil {
		return ""
	}
	tag, err := x.Get(goexif.ColorSpace)
	if err != nil {
		return ""
	}
	v, err := tag.Int(0)
	if err != nil {
		return ""
	}
	switch v {
	case 1:
		return "sRGB"
	case 0xFFFF:
		return "Uncalibrated"
	}
	return ""
}
