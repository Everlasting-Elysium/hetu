package psd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func TestMatch(t *testing.T) {
	h := New()
	if !h.Match("psd") {
		t.Error("Match(psd) = false, want true")
	}
	for _, ext := range []string{"psb", "png", "jpg", "tiff", ""} {
		if h.Match(ext) {
			t.Errorf("Match(%q) = true, want false", ext)
		}
	}
}

func TestKind(t *testing.T) {
	if got := New().Kind(); got != domain.KindImage {
		t.Fatalf("Kind() = %q, want %q", got, domain.KindImage)
	}
}

func TestExtractDimensions(t *testing.T) {
	meta, err := New().Extract(context.Background(), bytes.NewReader(flatPSD(8, 6)))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if meta.Kind != domain.KindImage {
		t.Fatalf("Kind = %q, want %q", meta.Kind, domain.KindImage)
	}
	if meta.Width != 8 || meta.Height != 6 {
		t.Fatalf("dims = %dx%d, want 8x6", meta.Width, meta.Height)
	}
}

// TestExtractMalformedDegrades asserts a non-PSD input is still indexed as
// kind=image with zero dimensions rather than failing the scan.
func TestExtractMalformedDegrades(t *testing.T) {
	meta, err := New().Extract(context.Background(), bytes.NewReader([]byte("not a psd")))
	if err != nil {
		t.Fatalf("Extract on garbage should not error, got %v", err)
	}
	if meta.Kind != domain.KindImage {
		t.Fatalf("Kind = %q, want %q", meta.Kind, domain.KindImage)
	}
}

func TestThumbnailFromMergedImage(t *testing.T) {
	var buf bytes.Buffer
	if err := New().Thumbnail(context.Background(), bytes.NewReader(flatPSD(16, 12)), &buf); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("thumbnail is not JPEG (len=%d)", buf.Len())
	}
}

// TestThumbnailNoMergedImageDegrades asserts a PSD without a merged image
// section yields ErrNoThumbnail (hetu never composites layers itself).
func TestThumbnailNoMergedImageDegrades(t *testing.T) {
	err := New().Thumbnail(context.Background(), bytes.NewReader(psdNoMergedImage(8, 8)), io.Discard)
	if !errors.Is(err, domain.ErrNoThumbnail) {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
}

func TestExtractMetadata(t *testing.T) {
	h := New()
	cases := []struct {
		name  string
		data  []byte
		count int
	}{
		{"flattened", flatPSD(8, 8), 0},
		{"three layers", layeredPSD(8, 8, 3), 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md, err := h.ExtractMetadata(context.Background(), bytes.NewReader(tc.data))
			if err != nil {
				t.Fatalf("ExtractMetadata: %v", err)
			}
			if got := md.Annotations[domain.KeyPSDLayerCount]; got != tc.count {
				t.Fatalf("layer_count = %v, want %d", got, tc.count)
			}
			if got := md.Annotations[domain.KeyPSDColorMode]; got != "RGB" {
				t.Fatalf("color_mode = %v, want RGB", got)
			}
			if got := md.Annotations[domain.KeyPSDBitDepth]; got != 8 {
				t.Fatalf("bit_depth = %v, want 8", got)
			}
		})
	}
}
