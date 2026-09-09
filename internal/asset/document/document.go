// Package document implements kernel.AssetHandler for documents: PDFs rendered
// natively, and PowerPoint files (ppt/pptx) rendered after a LibreOffice
// headless conversion to PDF. It renders pages to JPEG thumbnails via external
// tools (pdftoppm from poppler, or mutool from mupdf; soffice/libreoffice for
// office files), keeping the kernel CGO-free. When a tool is absent the handler
// degrades gracefully: documents are still indexed as kind=document, just
// without a thumbnail or per-page previews. It also implements
// kernel.PageExtractor (PageCount/RenderPage) so the indexer can build a
// per-page thumbnail index for multi-page documents (issue #48).
package document

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

const (
	// thumbMaxDim is the longest edge (px) of a generated thumbnail.
	thumbMaxDim  = 512
	thumbTimeout = 60 * time.Second
)

var supported = map[string]struct{}{"pdf": {}, "ppt": {}, "pptx": {}}

// renderer identifies which external PDF tool was found on PATH.
type renderer int

const (
	rendererNone renderer = iota
	rendererPdftoppm
	rendererMutool
)

// Handler processes PDF and PowerPoint documents via external tools.
type Handler struct {
	renderer renderer
	bin      string // resolved PATH of the chosen PDF renderer
	infoBin  string // resolved PATH of pdfinfo (poppler), for page counts
	soffice  string // resolved PATH of soffice/libreoffice, for ppt/pptx -> pdf
	log      *slog.Logger
	warn     sync.Once
}

var (
	_ kernel.AssetHandler  = (*Handler)(nil)
	_ kernel.PageExtractor = (*Handler)(nil)
	_ PDFPageRenderer      = (*Handler)(nil)
)

// PDFPageRenderer renders a single (1-based) page of a PDF-compatible file to w.
// It is exported so other handlers (e.g. design.Handler for .ai files, which are
// PDF-compatible) can reuse the same pdftoppm/mutool detection without
// duplicating subprocess logic.
type PDFPageRenderer interface {
	RenderPage(ctx context.Context, src io.ReadSeeker, page int, w io.Writer) error
}

// New returns a document handler, preferring pdftoppm then mutool on PATH for
// PDF rendering, pdfinfo for page counts, and soffice/libreoffice for converting
// office files (ppt/pptx) to PDF. Every tool is optional: absent one the
// affected documents are still indexed, just without a thumbnail or pages. A nil
// log falls back to slog.Default.
func New(log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	h := &Handler{log: log}
	if p, err := exec.LookPath("pdftoppm"); err == nil {
		h.renderer, h.bin = rendererPdftoppm, p
		h.infoBin, _ = exec.LookPath("pdfinfo")
	} else if p, err := exec.LookPath("mutool"); err == nil {
		h.renderer, h.bin = rendererMutool, p
	}
	if p, err := exec.LookPath("soffice"); err == nil {
		h.soffice = p
	} else if p, err := exec.LookPath("libreoffice"); err == nil {
		h.soffice = p
	}
	return h
}

// Match reports whether ext is a supported document extension.
func (h *Handler) Match(ext string) bool {
	_, ok := supported[ext]
	return ok
}

// Kind returns domain.KindDocument.
func (h *Handler) Kind() domain.AssetKind { return domain.KindDocument }

// Extract returns kind only: a document has no meaningful pixel dimensions until
// a page is rendered, so this is a best-effort, never-failing extraction.
func (h *Handler) Extract(_ context.Context, _ io.ReadSeeker) (domain.Meta, error) {
	return domain.Meta{Kind: domain.KindDocument}, nil
}

// Thumbnail renders the first page as JPEG into w (via RenderPage), or returns
// domain.ErrNoThumbnail if no renderer is available or rendering fails.
func (h *Handler) Thumbnail(ctx context.Context, src io.ReadSeeker, w io.Writer) error {
	return h.RenderPage(ctx, src, 1, w)
}

func (h *Handler) warnMissing(ctx context.Context) {
	h.warn.Do(func() {
		h.log.WarnContext(ctx,
			"pdftoppm/mutool not found on PATH; document thumbnails and pages disabled")
	})
}
