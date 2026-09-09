package document

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

// toolHandler returns a Handler wired to the real pdftoppm(+pdfinfo) or mutool on
// PATH (via New's detection), or skips when none is installed.
func toolHandler(t *testing.T) *Handler {
	t.Helper()
	h := New(nil)
	if h.renderer == rendererNone {
		t.Skip("neither pdftoppm nor mutool installed")
	}
	return h
}

// TestPrepareAndRenderWithTool exercises the real multi-page pipeline end to end:
// prepare once, count the pages, and render each to a valid JPEG. This is the
// path the stdout-output bug broke; it proves the file-output fix works with real
// tools and that Prepare renders every page from one preparation.
func TestPrepareAndRenderWithTool(t *testing.T) {
	h := toolHandler(t)
	ctx := context.Background()
	doc, err := h.Prepare(ctx, bytes.NewReader(multiPagePDF(3)))
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer func() { _ = doc.Close() }()

	count, err := doc.PageCount(ctx)
	if err != nil {
		if h.renderer == rendererPdftoppm {
			t.Fatalf("PageCount: %v", err)
		}
		t.Skipf("PageCount unavailable for this tool build: %v", err)
	}
	if count != 3 {
		t.Fatalf("PageCount = %d, want 3", count)
	}
	for page := 1; page <= count; page++ {
		var buf bytes.Buffer
		if err := doc.RenderPage(ctx, page, &buf); err != nil {
			t.Fatalf("RenderPage(%d): %v", page, err)
		}
		if !bytes.HasPrefix(buf.Bytes(), []byte{0xFF, 0xD8, 0xFF}) {
			t.Fatalf("page %d is not JPEG (len=%d)", page, buf.Len())
		}
	}
}

// TestThumbnailRendersWithTool proves the single-shot Thumbnail path (the asset's
// main thumbnail, and design's .ai preview) renders page 1 to JPEG with real
// tools — the exact scenario the stdout bug broke.
func TestThumbnailRendersWithTool(t *testing.T) {
	h := toolHandler(t)
	var buf bytes.Buffer
	if err := h.Thumbnail(context.Background(), bytes.NewReader(multiPagePDF(2)), &buf); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("thumbnail is not JPEG (len=%d)", buf.Len())
	}
}

// multiPagePDF builds a valid n-page PDF (blank pages, distinct MediaBoxes) with
// a correct xref table so strict renderers accept it without repair.
func multiPagePDF(n int) []byte {
	var b bytes.Buffer
	var offsets []int
	obj := func(body string) { offsets = append(offsets, b.Len()); b.WriteString(body) }
	b.WriteString("%PDF-1.4\n")
	obj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	kids := ""
	for i := range n {
		kids += fmt.Sprintf("%d 0 R ", i+3)
	}
	obj(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", kids, n))
	for i := range n {
		obj(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d 200] >>\nendobj\n", i+3, 200+i*50))
	}
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
