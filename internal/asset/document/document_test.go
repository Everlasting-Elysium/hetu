package document

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func discardHandler(r renderer, bin string) *Handler {
	return &Handler{renderer: r, bin: bin, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestMatch(t *testing.T) {
	h := discardHandler(rendererNone, "")
	if !h.Match("pdf") {
		t.Error("Match(pdf) = false, want true")
	}
	for _, ext := range []string{"jpg", "mp4", "txt", "docx", ""} {
		if h.Match(ext) {
			t.Errorf("Match(%q) = true, want false", ext)
		}
	}
}

func TestKind(t *testing.T) {
	if got := discardHandler(rendererNone, "").Kind(); got != domain.KindDocument {
		t.Fatalf("Kind() = %q, want %q", got, domain.KindDocument)
	}
}

func TestExtractReturnsKindOnly(t *testing.T) {
	meta, err := discardHandler(rendererNone, "").Extract(context.Background(), bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if meta.Kind != domain.KindDocument {
		t.Fatalf("Kind = %q, want %q", meta.Kind, domain.KindDocument)
	}
}

func TestThumbnailDegradesWithoutTool(t *testing.T) {
	h := discardHandler(rendererNone, "")
	err := h.Thumbnail(context.Background(), bytes.NewReader(minimalPDF()), io.Discard)
	if err != domain.ErrNoThumbnail {
		t.Fatalf("Thumbnail err = %v, want ErrNoThumbnail", err)
	}
}

// TestThumbnailWithTool exercises the real subprocess path. It is skipped when
// neither pdftoppm nor mutool is installed so CI stays green.
func TestThumbnailWithTool(t *testing.T) {
	var h *Handler
	if p, err := exec.LookPath("pdftoppm"); err == nil {
		h = discardHandler(rendererPdftoppm, p)
	} else if p, err := exec.LookPath("mutool"); err == nil {
		h = discardHandler(rendererMutool, p)
	} else {
		t.Skip("neither pdftoppm nor mutool installed")
	}

	var buf bytes.Buffer
	if err := h.Thumbnail(context.Background(), bytes.NewReader(minimalPDF()), &buf); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("thumbnail is not JPEG (len=%d)", buf.Len())
	}
}

// minimalPDF builds a valid single blank-page PDF with a correct xref table so
// strict renderers accept it without repair.
func minimalPDF() []byte {
	var b bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, b.Len())
		b.WriteString(body)
	}
	b.WriteString("%PDF-1.4\n")
	obj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	obj("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	obj("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n")
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(offsets)+1)
	b.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets)+1, xref)
	return b.Bytes()
}
