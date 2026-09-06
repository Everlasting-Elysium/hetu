package image

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func discardPro() *ProHandler {
	return &ProHandler{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestProMatch(t *testing.T) {
	h := discardPro()
	for _, ext := range []string{"cr2", "nef", "arw", "dng", "raf", "orf", "heic", "heif", "exr"} {
		if !h.Match(ext) {
			t.Errorf("Match(%q) = false, want true", ext)
		}
	}
	// Must not steal the pure-Go handler's extensions or unrelated types.
	for _, ext := range []string{"jpg", "jpeg", "png", "gif", "bmp", "tiff", "mp4", "pdf", ""} {
		if h.Match(ext) {
			t.Errorf("Match(%q) = true, want false", ext)
		}
	}
}

func TestProKind(t *testing.T) {
	if got := discardPro().Kind(); got != domain.KindImage {
		t.Fatalf("Kind() = %q, want %q", got, domain.KindImage)
	}
}

func TestSniffProFormat(t *testing.T) {
	tests := []struct {
		name string
		head []byte
		want proFormat
	}{
		{"exr magic", []byte{0x76, 0x2f, 0x31, 0x01, 0, 0}, fmtEXR},
		{"heic ftyp", []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, fmtHEIC},
		{"heif ftyp mif1", []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'm', 'i', 'f', '1'}, fmtHEIC},
		{"tiff little-endian raw", []byte{'I', 'I', 0x2a, 0x00, 0x10, 0, 0, 0}, fmtRAW},
		{"tiff big-endian raw", []byte{'M', 'M', 0x00, 0x2a, 0, 0, 0, 0x08}, fmtRAW},
		{"short buffer", []byte{0x01}, fmtRAW},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sniffProFormat(tc.head); got != tc.want {
				t.Fatalf("sniffProFormat = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestProThumbnailDegradesWithoutTools(t *testing.T) {
	h := discardPro() // no tools resolved
	exr := []byte{0x76, 0x2f, 0x31, 0x01, 0, 0, 0, 0}
	err := h.Thumbnail(context.Background(), bytes.NewReader(exr), io.Discard)
	if err != domain.ErrNoThumbnail {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
}

func TestProExtractDegradesGracefully(t *testing.T) {
	h := discardPro()
	meta, err := h.Extract(context.Background(), bytes.NewReader([]byte("not a real raw file")))
	if err != nil {
		t.Fatalf("Extract must not fail the scan: %v", err)
	}
	if meta.Kind != domain.KindImage {
		t.Fatalf("Kind = %q, want %q", meta.Kind, domain.KindImage)
	}
	if meta.Width != 0 || meta.Height != 0 {
		t.Fatalf("dims = %dx%d, want 0x0 when EXIF absent", meta.Width, meta.Height)
	}
}

func TestProExtractMetadataDegradesGracefully(t *testing.T) {
	h := discardPro()
	md, err := h.ExtractMetadata(context.Background(), bytes.NewReader([]byte("garbage")))
	if err != nil {
		t.Fatalf("ExtractMetadata must not fail: %v", err)
	}
	if len(md.Annotations) != 0 {
		t.Fatalf("annotations = %v, want empty for unreadable input", md.Annotations)
	}
}

// TestProThumbnailEXRWithFFmpeg exercises the real EXR decode path. ffmpeg
// decodes OpenEXR natively, so this runs wherever ffmpeg is installed and is
// skipped otherwise to keep CI green.
func TestProThumbnailEXRWithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	sample := filepath.Join(t.TempDir(), "sample.exr")
	gen := exec.Command("ffmpeg", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=160x120:rate=1",
		"-frames:v", "1", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("cannot generate EXR sample (ffmpeg lacks exr encoder?): %v\n%s", err, out)
	}

	h := NewPro(slog.New(slog.NewTextHandler(io.Discard, nil)))
	f, err := os.Open(sample)
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	if err := h.Thumbnail(context.Background(), f, &buf); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("thumbnail is not JPEG (len=%d)", buf.Len())
	}
}
