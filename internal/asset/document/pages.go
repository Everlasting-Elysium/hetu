package document

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/Everlasting-Elysium/hetu/internal/asset/mediaproc"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// pdfMagic is the 5-byte prefix of a PDF (and a PDF-compatible .ai) file.
const pdfMagic = "%PDF-"

// Prepare implements kernel.PageExtractor: it ensures a local PDF exists for src
// — converting an office file, or copying a PDF, exactly once — and returns a
// preparedDoc whose PageCount and RenderPage reuse that single PDF, so paging an
// N-page document costs one conversion, not N. The caller must Close it. A
// missing renderer yields domain.ErrNoThumbnail so the indexer skips paging.
func (h *Handler) Prepare(ctx context.Context, src io.ReadSeeker) (kernel.PagedDocument, error) {
	if h.renderer == rendererNone {
		h.warnMissing(ctx)
		return nil, domain.ErrNoThumbnail
	}
	path, cleanup, err := h.ensurePDF(ctx, src)
	if err != nil {
		h.log.DebugContext(ctx, "prepare document failed", slog.Any("err", err))
		return nil, domain.ErrNoThumbnail
	}
	return &preparedDoc{h: h, path: path, cleanup: cleanup}, nil
}

// preparedDoc is a single prepared PDF (native, or converted from an office file
// once) that pages are counted and rendered from without re-converting. It
// implements kernel.PagedDocument.
type preparedDoc struct {
	h       *Handler
	path    string
	cleanup func()
}

var _ kernel.PagedDocument = (*preparedDoc)(nil)

// PageCount returns the prepared PDF's page count.
func (d *preparedDoc) PageCount(ctx context.Context) (int, error) {
	return d.h.countPages(ctx, d.path)
}

// RenderPage renders the 1-based page of the prepared PDF as JPEG into w.
func (d *preparedDoc) RenderPage(ctx context.Context, page int, w io.Writer) error {
	return d.h.writeRenderedPage(ctx, d.path, page, w)
}

// Close removes the prepared PDF (and any converted temp files).
func (d *preparedDoc) Close() error {
	d.cleanup()
	return nil
}

// RenderPage renders a single (1-based) page of a PDF-compatible source as JPEG
// into w for a one-shot preview — the asset's main thumbnail, and design's .ai
// preview — preparing once and rendering once. Multi-page indexing uses Prepare
// to avoid re-preparing per page. It implements PDFPageRenderer. Any failure
// yields domain.ErrNoThumbnail so a scan never blocks on a document.
func (h *Handler) RenderPage(ctx context.Context, src io.ReadSeeker, page int, w io.Writer) error {
	if h.renderer == rendererNone {
		h.warnMissing(ctx)
		return domain.ErrNoThumbnail
	}
	path, cleanup, err := h.ensurePDF(ctx, src)
	if err != nil {
		h.log.DebugContext(ctx, "ensure pdf for render failed", slog.Any("err", err))
		return domain.ErrNoThumbnail
	}
	defer cleanup()
	return h.writeRenderedPage(ctx, path, page, w)
}

// writeRenderedPage renders page from the PDF at path and writes the JPEG to w,
// or returns domain.ErrNoThumbnail when rendering fails or yields no bytes.
func (h *Handler) writeRenderedPage(ctx context.Context, path string, page int, w io.Writer) error {
	out, err := h.renderPage(ctx, path, page)
	if err != nil {
		h.log.DebugContext(ctx, "document render page failed",
			slog.Int("page", page), slog.Any("err", err))
		return domain.ErrNoThumbnail
	}
	if len(out) == 0 {
		return domain.ErrNoThumbnail
	}
	if _, err := w.Write(out); err != nil {
		return fmt.Errorf("write page: %w", err)
	}
	return nil
}

// ensurePDF returns a filesystem path to a PDF for src, plus a cleanup func the
// caller must defer. A PDF (or PDF-compatible .ai) source is copied to a temp
// file as-is; any other (office) source is converted to PDF via LibreOffice.
func (h *Handler) ensurePDF(ctx context.Context, src io.ReadSeeker) (string, func(), error) {
	pdf, err := sniffPDF(src)
	if err != nil {
		return "", nil, err
	}
	if pdf {
		return mediaproc.TempCopy(src, ".pdf")
	}
	return h.convertOffice(ctx, src)
}

// sniffPDF reports whether src begins with the PDF magic, seeking back to the
// start so the caller can re-read from the beginning regardless of the result.
func sniffPDF(src io.ReadSeeker) (bool, error) {
	head := make([]byte, len(pdfMagic))
	n, readErr := io.ReadFull(src, head)
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return false, fmt.Errorf("seek source: %w", err)
	}
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		return false, fmt.Errorf("read header: %w", readErr)
	}
	return string(head[:n]) == pdfMagic, nil
}
