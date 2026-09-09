package font

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// The test fixture is the bundled Go font (golang.org/x/image/font/gofont),
// BSD-licensed and shipped with the golang.org/x/image module already in go.mod,
// so no binary fixture is vendored. Its family name is "Go".

func TestMatch(t *testing.T) {
	h := New()
	for _, ext := range []string{"ttf", "otf", "woff"} {
		if !h.Match(ext) {
			t.Errorf("Match(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{"woff2", "png", "pdf", ""} {
		if h.Match(ext) {
			t.Errorf("Match(%q) = true, want false", ext)
		}
	}
}

func TestKind(t *testing.T) {
	if got := New().Kind(); got != domain.KindFont {
		t.Fatalf("Kind() = %q, want %q", got, domain.KindFont)
	}
}

func TestExtractReturnsKindOnly(t *testing.T) {
	meta, err := New().Extract(context.Background(), bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if meta.Kind != domain.KindFont {
		t.Fatalf("Kind = %q, want %q", meta.Kind, domain.KindFont)
	}
	if meta.Width != 0 || meta.Height != 0 {
		t.Fatalf("dims = %dx%d, want 0x0", meta.Width, meta.Height)
	}
}

func TestThumbnailRendersJPEG(t *testing.T) {
	var buf bytes.Buffer
	if err := New().Thumbnail(context.Background(), bytes.NewReader(goregular.TTF), &buf); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("thumbnail is not JPEG (len=%d)", buf.Len())
	}
}

// TestThumbnailDegradesOnGarbage asserts an unparseable file yields
// ErrNoThumbnail rather than a hard failure.
func TestThumbnailDegradesOnGarbage(t *testing.T) {
	err := New().Thumbnail(context.Background(), bytes.NewReader([]byte("not a font")), io.Discard)
	if !errors.Is(err, domain.ErrNoThumbnail) {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
}

func TestExtractMetadataFamily(t *testing.T) {
	md, err := New().ExtractMetadata(context.Background(), bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatalf("ExtractMetadata: %v", err)
	}
	if got := md.Annotations[domain.KeyFontFamily]; got != "Go" {
		t.Fatalf("font.family = %v, want Go", got)
	}
	if _, ok := md.Annotations[domain.KeyFontStyle]; !ok {
		t.Fatal("font.style not extracted")
	}
	if _, ok := md.Annotations[domain.KeyFontWeight]; !ok {
		t.Fatal("font.weight not extracted")
	}
}

func TestWeightFromStyle(t *testing.T) {
	cases := map[string]string{
		"Regular": "Regular", "Bold": "Bold", "Bold Italic": "Bold",
		"SemiBold": "SemiBold", "ExtraLight": "ExtraLight", "Italic": "Regular",
	}
	for style, want := range cases {
		if got := weightFromStyle(style); got != want {
			t.Errorf("weightFromStyle(%q) = %q, want %q", style, got, want)
		}
	}
}
