package image

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// iccHeaderLen is the fixed ICC profile header size; the tag table follows it.
const iccHeaderLen = 128

// parseICCProfile extracts the data color-space signature (header bytes 16-19,
// e.g. "RGB") and the human-readable profile description (the 'desc' tag) from a
// raw ICC profile. It is pure so it can be unit-tested over byte fixtures, and
// returns empty strings for a malformed or truncated profile.
func parseICCProfile(p []byte) (space, desc string) {
	if len(p) < iccHeaderLen+4 || string(p[36:40]) != "acsp" {
		return "", ""
	}
	space = strings.TrimSpace(string(p[16:20]))
	tagCount := int(binary.BigEndian.Uint32(p[iccHeaderLen : iccHeaderLen+4]))
	table := iccHeaderLen + 4
	for i := 0; i < tagCount; i++ {
		off := table + i*12
		if off+12 > len(p) {
			break
		}
		if string(p[off:off+4]) != "desc" {
			continue
		}
		tOff := int(binary.BigEndian.Uint32(p[off+4 : off+8]))
		tLen := int(binary.BigEndian.Uint32(p[off+8 : off+12]))
		if tOff < 8 || tLen < 8 || tOff+tLen > len(p) {
			return space, ""
		}
		return space, parseDescTag(p[tOff : tOff+tLen])
	}
	return space, ""
}

// parseDescTag decodes an ICC 'desc' tag, supporting both the v2 textDescription
// ("desc") and the v4 multiLocalizedUnicode ("mluc") encodings.
func parseDescTag(t []byte) string {
	if len(t) < 12 {
		return ""
	}
	switch string(t[0:4]) {
	case "desc": // textDescription: type(4) reserved(4) count(4) ascii...
		n := int(binary.BigEndian.Uint32(t[8:12]))
		if n <= 0 || 12+n > len(t) {
			return ""
		}
		return strings.TrimRight(string(t[12:12+n]), "\x00")
	case "mluc": // multiLocalizedUnicode: type(4) reserved(4) count(4) size(4) rec...
		if len(t) < 28 {
			return ""
		}
		length := int(binary.BigEndian.Uint32(t[20:24]))
		offset := int(binary.BigEndian.Uint32(t[24:28]))
		if length <= 0 || offset < 28 || offset+length > len(t) {
			return ""
		}
		return decodeUTF16BE(t[offset : offset+length])
	}
	return ""
}

// decodeUTF16BE decodes big-endian UTF-16 bytes (ICC 'mluc' strings) to UTF-8.
func decodeUTF16BE(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.BigEndian.Uint16(b[i*2:])
	}
	return strings.TrimRight(string(utf16.Decode(u)), "\x00")
}

// normalizeColorSpace maps a verbose ICC description to a short, stable name.
// Unknown descriptions are returned trimmed so the UI still shows something.
func normalizeColorSpace(desc string) string {
	d := strings.ToLower(desc)
	switch {
	case strings.Contains(d, "adobe rgb"):
		return "Adobe RGB"
	case strings.Contains(d, "display p3"), strings.Contains(d, "displayp3"):
		return "Display P3"
	case strings.Contains(d, "prophoto"):
		return "ProPhoto RGB"
	case strings.Contains(d, "rec2020"), strings.Contains(d, "rec. 2020"), strings.Contains(d, "bt.2020"):
		return "Rec. 2020"
	case strings.Contains(d, "srgb"):
		return "sRGB"
	}
	return strings.TrimSpace(desc)
}
