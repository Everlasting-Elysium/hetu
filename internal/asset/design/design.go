// Package design implements kernel.AssetHandler for opaque design files —
// Adobe Illustrator (.ai), InDesign (.indd), Sketch (.sketch), Figma (.fig),
// and After Effects (.aep). All are indexed as kind=design; only some can be
// previewed: a PDF-compatible .ai is rendered via the shared PDF page renderer,
// and a .sketch bundle's embedded preview PNG is extracted. The rest are
// registered without a thumbnail (issue #48). Format is detected by content
// (magic bytes), like the professional image handler, because AssetHandler
// methods receive only a reader, not the extension.
package design

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Everlasting-Elysium/hetu/internal/asset/document"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// sniffLen is the number of leading bytes read to classify a design file.
const sniffLen = 5

var supported = map[string]struct{}{
	"ai": {}, "indd": {}, "sketch": {}, "fig": {}, "aep": {},
}

// format is the content-detected container of a design file.
type format int

const (
	formatOther format = iota
	formatPDF          // PDF-compatible (.ai) — render via the PDF renderer
	formatZip          // ZIP bundle (.sketch) — extract the embedded preview
)

// Handler processes opaque design files. It previews PDF-compatible .ai files
// through an injected PDF page renderer (the document handler) so pdftoppm/
// mutool detection is shared, and Sketch bundles by extracting their embedded
// preview — everything else is registered without a preview.
type Handler struct {
	pdf document.PDFPageRenderer
}

var _ kernel.AssetHandler = (*Handler)(nil)

// New returns a design handler. pdf renders PDF-compatible files (.ai); passing
// the shared document.Handler avoids duplicating pdftoppm/mutool detection. A
// nil pdf disables .ai previews (they degrade to no thumbnail).
func New(pdf document.PDFPageRenderer) *Handler {
	return &Handler{pdf: pdf}
}

// Match reports whether ext is a supported design extension.
func (h *Handler) Match(ext string) bool {
	_, ok := supported[ext]
	return ok
}

// Kind returns domain.KindDesign.
func (h *Handler) Kind() domain.AssetKind { return domain.KindDesign }

// Extract returns kind=design with no dimensions: these containers are opaque,
// so hetu never parses their canvas size.
func (h *Handler) Extract(_ context.Context, _ io.ReadSeeker) (domain.Meta, error) {
	return domain.Meta{Kind: domain.KindDesign}, nil
}

// Thumbnail previews the file when it is one hetu can read: a PDF-compatible .ai
// is rendered (page 1) via the injected PDF renderer, and a Sketch bundle's
// embedded preview is extracted. Detection is by content, so InDesign/Figma/
// AfterEffects (and any non-PDF .ai) return domain.ErrNoThumbnail.
func (h *Handler) Thumbnail(ctx context.Context, src io.ReadSeeker, w io.Writer) error {
	f, err := sniff(src)
	if err != nil {
		return domain.ErrNoThumbnail
	}
	switch f {
	case formatPDF:
		if h.pdf == nil {
			return domain.ErrNoThumbnail
		}
		// Any render failure (e.g. a non-PDF-compatible .ai) degrades to no
		// thumbnail rather than interrupting the scan.
		if err := h.pdf.RenderPage(ctx, src, 1, w); err != nil {
			return domain.ErrNoThumbnail
		}
		return nil
	case formatZip:
		return extractSketchPreview(src, w)
	case formatOther:
		return domain.ErrNoThumbnail
	}
	return domain.ErrNoThumbnail
}

// sniff classifies src by its leading magic bytes and rewinds it: "%PDF-" for a
// PDF-compatible file (.ai), the ZIP local-file signature for a bundle
// (.sketch), else formatOther.
func sniff(src io.ReadSeeker) (format, error) {
	head := make([]byte, sniffLen)
	n, readErr := io.ReadFull(src, head)
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return formatOther, fmt.Errorf("seek source: %w", err)
	}
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		return formatOther, fmt.Errorf("read header: %w", readErr)
	}
	head = head[:n]
	switch {
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return formatPDF, nil
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		return formatZip, nil
	}
	return formatOther, nil
}
