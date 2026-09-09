package design

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	stdimage "image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// stubRenderer is a fake document.PDFPageRenderer: it records the page it was
// asked to render and writes a marker payload, or fails when configured to.
type stubRenderer struct {
	called  bool
	page    int
	fail    bool
	payload []byte
}

func (s *stubRenderer) RenderPage(_ context.Context, _ io.ReadSeeker, page int, w io.Writer) error {
	s.called = true
	s.page = page
	if s.fail {
		return domain.ErrNoThumbnail
	}
	_, err := w.Write(s.payload)
	return err
}

func TestMatch(t *testing.T) {
	h := New(nil)
	for _, ext := range []string{"ai", "indd", "sketch", "fig", "aep"} {
		if !h.Match(ext) {
			t.Errorf("Match(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{"psd", "pdf", "png", ""} {
		if h.Match(ext) {
			t.Errorf("Match(%q) = true, want false", ext)
		}
	}
}

func TestKind(t *testing.T) {
	if got := New(nil).Kind(); got != domain.KindDesign {
		t.Fatalf("Kind() = %q, want %q", got, domain.KindDesign)
	}
}

func TestExtract(t *testing.T) {
	meta, err := New(nil).Extract(context.Background(), bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if meta.Kind != domain.KindDesign {
		t.Fatalf("Kind = %q, want %q", meta.Kind, domain.KindDesign)
	}
}

// TestThumbnailAIRendersViaRenderer asserts a PDF-compatible .ai is dispatched
// to the injected renderer for page 1 and its output is streamed through.
func TestThumbnailAIRendersViaRenderer(t *testing.T) {
	stub := &stubRenderer{payload: []byte{0xFF, 0xD8, 0xFF}}
	var buf bytes.Buffer
	src := bytes.NewReader([]byte("%PDF-1.5\n... illustrator ..."))
	if err := New(stub).Thumbnail(context.Background(), src, &buf); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	if !stub.called || stub.page != 1 {
		t.Fatalf("renderer called=%v page=%d, want true/1", stub.called, stub.page)
	}
	if !bytes.Equal(buf.Bytes(), stub.payload) {
		t.Fatalf("thumbnail = %v, want renderer payload", buf.Bytes())
	}
}

func TestThumbnailAIRendererFailureDegrades(t *testing.T) {
	err := New(&stubRenderer{fail: true}).Thumbnail(
		context.Background(), bytes.NewReader([]byte("%PDF-1.5")), io.Discard)
	if !errors.Is(err, domain.ErrNoThumbnail) {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
}

// TestThumbnailSketchExtractsPreview verifies a real, decodable preview entry
// is extracted, fit through the shared encoder, and re-emitted as a JPEG at
// the expected max dimension. The preview is intentionally larger than
// previewMaxDim on its long edge so the Fit step is actually exercised (a
// pass-through copy, which is what the pre-fix implementation did, would
// leave it as PNG and at the original size — this test would catch that
// regression too).
func TestThumbnailSketchExtractsPreview(t *testing.T) {
	previewPNG := makeTestPNG(t, 800, 400)
	data := sketchZip(t, sketchPreviewPath, previewPNG)
	var buf bytes.Buffer
	if err := New(nil).Thumbnail(context.Background(), bytes.NewReader(data), &buf); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("decode output as JPEG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != previewMaxDim || b.Dy() != previewMaxDim/2 {
		t.Fatalf("output size = %dx%d, want %dx%d", b.Dx(), b.Dy(), previewMaxDim, previewMaxDim/2)
	}
}

// TestThumbnailSketchOversizedPreviewDegrades verifies a preview entry whose
// declared dimensions exceed the decode sanity limit is rejected before any
// pixel buffer is allocated (defuses a dimension-bomb PNG).
func TestThumbnailSketchOversizedPreviewDegrades(t *testing.T) {
	huge := fakePNGHeader(t, 100000, 100000)
	data := sketchZip(t, sketchPreviewPath, huge)
	err := New(nil).Thumbnail(context.Background(), bytes.NewReader(data), io.Discard)
	if !errors.Is(err, domain.ErrNoThumbnail) {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
}

// makeTestPNG encodes a real, decodable w x h PNG for use as a sketch preview
// fixture.
func makeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 0x80, A: 0xFF})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture png: %v", err)
	}
	return buf.Bytes()
}

// fakePNGHeader builds a structurally valid PNG (correct signature + IHDR
// chunk + CRC) that declares w x h dimensions without any real pixel data
// after it, so image.DecodeConfig reports the declared size without a real
// decode ever needing to allocate w*h pixels.
func fakePNGHeader(t *testing.T, w, h int) []byte {
	t.Helper()
	// Real 1x1 PNG, then patch the IHDR width/height fields and recompute the
	// IHDR chunk's CRC so the header parses as declaring w x h.
	base := makeTestPNG(t, 1, 1)
	// PNG layout: 8-byte signature, then chunks of [4-byte length][4-byte
	// type][data][4-byte crc]. IHDR is always the first chunk, right after
	// the signature, with data = width(4) height(4) bitdepth(1) ... (13 bytes).
	const sigLen = 8
	const lenFieldLen = 4
	const typeFieldLen = 4
	ihdrData := sigLen + lenFieldLen + typeFieldLen
	putUint32BE(base[ihdrData:], uint32(w))
	putUint32BE(base[ihdrData+4:], uint32(h))
	crc := crc32.ChecksumIEEE(base[sigLen+lenFieldLen : ihdrData+13]) // type+data
	putUint32BE(base[ihdrData+13:], crc)
	return base
}

func putUint32BE(b []byte, v uint32) { binary.BigEndian.PutUint32(b, v) }

func TestThumbnailSketchNoPreviewDegrades(t *testing.T) {
	data := sketchZip(t, "pages/page1.json", []byte("{}"))
	err := New(nil).Thumbnail(context.Background(), bytes.NewReader(data), io.Discard)
	if !errors.Is(err, domain.ErrNoThumbnail) {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
}

// TestThumbnailOpaqueFormatsDegrade covers indd/fig/aep (neither PDF nor ZIP):
// they are registered without a preview.
func TestThumbnailOpaqueFormatsDegrade(t *testing.T) {
	for _, head := range [][]byte{
		{0x06, 0x06, 0xED, 0xF5}, // InDesign
		{0x00, 0x01, 0x02, 0x03}, // arbitrary fig/aep stand-in
	} {
		err := New(&stubRenderer{}).Thumbnail(context.Background(), bytes.NewReader(head), io.Discard)
		if !errors.Is(err, domain.ErrNoThumbnail) {
			t.Fatalf("Thumbnail(%v) err = %v, want ErrNoThumbnail", head, err)
		}
	}
}

// sketchZip builds an in-memory ZIP (a minimal Sketch bundle) with one entry.
func sketchZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
