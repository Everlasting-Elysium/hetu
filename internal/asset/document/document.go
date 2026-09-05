// Package document implements kernel.AssetHandler for documents (currently
// PDF). It renders the first page to a JPEG thumbnail via an external tool
// (pdftoppm from poppler, or mutool from mupdf), keeping the kernel CGO-free.
// When neither tool is present the handler degrades gracefully: PDFs are still
// indexed as kind=document, just without a thumbnail.
package document

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/asset/mediaproc"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

const (
	// thumbMaxDim is the longest edge (px) of a generated thumbnail.
	thumbMaxDim  = 512
	thumbTimeout = 60 * time.Second
)

var supported = map[string]struct{}{"pdf": {}}

// renderer identifies which external PDF tool was found on PATH.
type renderer int

const (
	rendererNone renderer = iota
	rendererPdftoppm
	rendererMutool
)

// Handler processes PDF documents via an external renderer.
type Handler struct {
	renderer renderer
	bin      string // resolved PATH of the chosen tool
	log      *slog.Logger
	warn     sync.Once
}

var _ kernel.AssetHandler = (*Handler)(nil)

// New returns a document handler, preferring pdftoppm then mutool on PATH. A
// nil log falls back to slog.Default.
func New(log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if p, err := exec.LookPath("pdftoppm"); err == nil {
		return &Handler{renderer: rendererPdftoppm, bin: p, log: log}
	}
	if p, err := exec.LookPath("mutool"); err == nil {
		return &Handler{renderer: rendererMutool, bin: p, log: log}
	}
	return &Handler{renderer: rendererNone, log: log}
}

// Match reports whether ext is a supported document extension.
func (h *Handler) Match(ext string) bool {
	_, ok := supported[ext]
	return ok
}

// Kind returns domain.KindDocument.
func (h *Handler) Kind() domain.AssetKind { return domain.KindDocument }

// Extract returns kind only: a PDF has no meaningful pixel dimensions until a
// page is rendered, so this is a best-effort, never-failing extraction.
func (h *Handler) Extract(_ context.Context, _ io.ReadSeeker) (domain.Meta, error) {
	return domain.Meta{Kind: domain.KindDocument}, nil
}

// Thumbnail renders the first PDF page as JPEG into w, or returns
// domain.ErrNoThumbnail if no renderer is available or rendering fails.
func (h *Handler) Thumbnail(ctx context.Context, src io.ReadSeeker, w io.Writer) error {
	if h.renderer == rendererNone {
		h.warnMissing(ctx)
		return domain.ErrNoThumbnail
	}
	path, cleanup, err := mediaproc.TempCopy(src, ".pdf")
	if err != nil {
		h.log.DebugContext(ctx, "pdf temp copy failed", slog.Any("err", err))
		return domain.ErrNoThumbnail
	}
	defer cleanup()

	out, err := h.render(ctx, path)
	if err != nil {
		h.log.DebugContext(ctx, "pdf thumbnail failed", slog.Any("err", err))
		return domain.ErrNoThumbnail
	}
	if len(out) == 0 {
		return domain.ErrNoThumbnail
	}
	if _, err := w.Write(out); err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}
	return nil
}

// render invokes the chosen tool on the first page and returns JPEG bytes.
func (h *Handler) render(ctx context.Context, path string) ([]byte, error) {
	switch h.renderer {
	case rendererPdftoppm:
		// pdftoppm streams the single rendered page to stdout ("-").
		return mediaproc.Run(ctx, thumbTimeout, h.bin,
			"-jpeg", "-singlefile", "-f", "1", "-l", "1",
			"-scale-to", strconv.Itoa(thumbMaxDim), path, "-")
	case rendererMutool:
		return h.renderMutool(ctx, path)
	case rendererNone:
		return nil, domain.ErrNoThumbnail
	}
	return nil, domain.ErrNoThumbnail
}

// renderMutool draws page 1 to a temp JPEG then reads it back: mutool's stdout
// support varies across builds, so a file target is the portable path.
func (h *Handler) renderMutool(ctx context.Context, path string) ([]byte, error) {
	outPath := path + ".jpg"
	defer func() { _ = os.Remove(outPath) }()
	if _, err := mediaproc.Run(ctx, thumbTimeout, h.bin,
		"draw", "-o", outPath, "-w", strconv.Itoa(thumbMaxDim), path, "1"); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("read mutool output: %w", err)
	}
	return data, nil
}

func (h *Handler) warnMissing(ctx context.Context) {
	h.warn.Do(func() {
		h.log.WarnContext(ctx,
			"pdftoppm/mutool not found on PATH; PDF thumbnails disabled")
	})
}
