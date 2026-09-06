package image

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// buildICC assembles a minimal but valid ICC profile carrying one textDescription
// ('desc') tag, enough to exercise parseICCProfile without a binary fixture.
func buildICC(space, desc string) []byte {
	ascii := append([]byte(desc), 0) // NUL-terminated per the 'desc' type
	descTag := make([]byte, 12+len(ascii))
	copy(descTag[0:4], "desc")
	binary.BigEndian.PutUint32(descTag[8:12], uint32(len(ascii)))
	copy(descTag[12:], ascii)

	header := make([]byte, iccHeaderLen)
	copy(header[16:20], space)
	copy(header[36:40], "acsp")

	count := make([]byte, 4)
	binary.BigEndian.PutUint32(count, 1)
	entry := make([]byte, 12)
	tagOffset := iccHeaderLen + 4 + 12
	copy(entry[0:4], "desc")
	binary.BigEndian.PutUint32(entry[4:8], uint32(tagOffset))
	binary.BigEndian.PutUint32(entry[8:12], uint32(len(descTag)))

	out := append([]byte{}, header...)
	out = append(out, count...)
	out = append(out, entry...)
	out = append(out, descTag...)
	return out
}

func TestParseICCProfile(t *testing.T) {
	space, desc := parseICCProfile(buildICC("RGB ", "Display P3"))
	if space != "RGB" {
		t.Errorf("space = %q, want %q", space, "RGB")
	}
	if desc != "Display P3" {
		t.Errorf("desc = %q, want %q", desc, "Display P3")
	}

	if _, d := parseICCProfile([]byte("too short")); d != "" {
		t.Errorf("malformed profile desc = %q, want empty", d)
	}
	if _, d := parseICCProfile(nil); d != "" {
		t.Errorf("nil profile desc = %q, want empty", d)
	}
}

func TestNormalizeColorSpace(t *testing.T) {
	tests := map[string]string{
		"Display P3":          "Display P3",
		"Adobe RGB (1998)":    "Adobe RGB",
		"sRGB IEC61966-2.1":   "sRGB",
		"ProPhoto RGB":        "ProPhoto RGB",
		"Rec2020":             "Rec. 2020",
		"Some Custom Profile": "Some Custom Profile",
	}
	for in, want := range tests {
		if got := normalizeColorSpace(in); got != want {
			t.Errorf("normalizeColorSpace(%q) = %q, want %q", in, got, want)
		}
	}
}

// buildPNGWithICC produces a minimal PNG (signature + iCCP + IEND) whose iCCP
// chunk carries the zlib-compressed profile, exercising the PNG locator path.
func buildPNGWithICC(icc []byte) []byte {
	var payload bytes.Buffer
	payload.WriteString("icc") // profile name
	payload.WriteByte(0)       // name terminator
	payload.WriteByte(0)       // compression method: zlib
	zw := zlib.NewWriter(&payload)
	_, _ = zw.Write(icc)
	_ = zw.Close()

	var buf bytes.Buffer
	buf.WriteString(pngSignature)
	writePNGChunk(&buf, "iCCP", payload.Bytes())
	writePNGChunk(&buf, "IEND", nil)
	return buf.Bytes()
}

func writePNGChunk(buf *bytes.Buffer, ctype string, data []byte) {
	var lenb [4]byte
	binary.BigEndian.PutUint32(lenb[:], uint32(len(data)))
	buf.Write(lenb[:])
	buf.WriteString(ctype)
	buf.Write(data)
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte(ctype))
	_, _ = crc.Write(data)
	var crcb [4]byte
	binary.BigEndian.PutUint32(crcb[:], crc.Sum32())
	buf.Write(crcb[:])
}

func TestPNGICCProfileRoundTrip(t *testing.T) {
	icc := buildICC("RGB ", "Adobe RGB (1998)")
	png := buildPNGWithICC(icc)

	got := pngICCProfile(bytes.NewReader(png))
	if !bytes.Equal(got, icc) {
		t.Fatalf("pngICCProfile returned %d bytes, want %d identical", len(got), len(icc))
	}
}

func TestExtractColorSpaceFromPNG(t *testing.T) {
	png := buildPNGWithICC(buildICC("RGB ", "Adobe RGB (1998)"))
	md := domain.ExtractedMetadata{Annotations: make(map[string]any)}
	extractColorSpace(bytes.NewReader(png), &md)

	if got := md.Annotations[domain.KeyColorICCProfile]; got != "Adobe RGB (1998)" {
		t.Errorf("icc_profile = %v, want %q", got, "Adobe RGB (1998)")
	}
	if got := md.Annotations[domain.KeyColorSpace]; got != "Adobe RGB" {
		t.Errorf("color.space = %v, want %q", got, "Adobe RGB")
	}
}

func TestExtractColorSpaceNoProfile(t *testing.T) {
	md := domain.ExtractedMetadata{Annotations: make(map[string]any)}
	extractColorSpace(bytes.NewReader([]byte("not an image")), &md)
	if len(md.Annotations) != 0 {
		t.Fatalf("annotations = %v, want empty when no profile present", md.Annotations)
	}
}

// TestPNGICCProfileRejectsHugeChunk guards the DoS fix: a non-iCCP chunk that
// declares a ~2GB length but carries almost no data must be streamed past and
// return nil fast, never triggering a giant make([]byte, length).
func TestPNGICCProfileRejectsHugeChunk(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(pngSignature)
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], 0x7FFFFFFF) // attacker-declared huge length
	copy(hdr[4:8], "tEXt")
	buf.Write(hdr[:])
	buf.WriteString("tiny") // far fewer bytes than declared -> hits EOF

	if got := pngICCProfile(bytes.NewReader(buf.Bytes())); got != nil {
		t.Fatalf("expected nil for truncated oversized chunk, got %d bytes", len(got))
	}
}

// TestPNGICCProfileRejectsOversizedICCP guards the iCCP size cap.
func TestPNGICCProfileRejectsOversizedICCP(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(pngSignature)
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(maxICCSize+1))
	copy(hdr[4:8], "iCCP")
	buf.Write(hdr[:])

	if got := pngICCProfile(bytes.NewReader(buf.Bytes())); got != nil {
		t.Fatalf("expected nil for oversized iCCP chunk, got %d bytes", len(got))
	}
}

func TestAssembleICCChunks(t *testing.T) {
	// Out-of-order chunks must reassemble in ascending sequence order.
	chunks := map[int][]byte{2: []byte("world"), 1: []byte("hello")}
	if got := string(assembleICCChunks(chunks)); got != "helloworld" {
		t.Fatalf("assembleICCChunks = %q, want %q", got, "helloworld")
	}
	if assembleICCChunks(nil) != nil {
		t.Fatal("empty chunks should return nil")
	}
	if assembleICCChunks(map[int][]byte{1: []byte("a"), 3: []byte("c")}) == nil {
		t.Fatal("a gap must still return the partial prefix, not nil/panic")
	}
}
