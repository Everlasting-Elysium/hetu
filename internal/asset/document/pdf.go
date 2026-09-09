package document

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/asset/mediaproc"
	"github.com/Everlasting-Elysium/hetu/internal/asset/thumb"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

const probeTimeout = 30 * time.Second

// renderPage renders the 1-based page of the PDF at path to JPEG bytes using the
// resolved external tool. Both tools write to a temp file rather than stdout:
// pdftoppm's stdout output ("-") yields zero bytes on modern poppler (26.x), and
// mudraw cannot emit JPEG at all, so a file target is the portable choice.
func (h *Handler) renderPage(ctx context.Context, path string, page int) ([]byte, error) {
	switch h.renderer {
	case rendererPdftoppm:
		return h.renderPdftoppm(ctx, path, page)
	case rendererMutool:
		return h.renderMutool(ctx, path, page)
	case rendererNone:
		return nil, domain.ErrNoThumbnail
	}
	return nil, domain.ErrNoThumbnail
}

// renderPdftoppm renders page to "<path>.p<page>.jpg" with -singlefile (a
// predictable, digit-free output name) and reads it back. -singlefile still
// honors -f/-l, so page N is selected correctly.
func (h *Handler) renderPdftoppm(ctx context.Context, path string, page int) ([]byte, error) {
	pg := strconv.Itoa(page)
	prefix := fmt.Sprintf("%s.p%d", path, page)
	outPath := prefix + ".jpg"
	defer func() { _ = os.Remove(outPath) }()
	if _, err := mediaproc.Run(ctx, thumbTimeout, h.bin,
		"-jpeg", "-singlefile", "-f", pg, "-l", pg,
		"-scale-to", strconv.Itoa(thumbMaxDim), path, prefix); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("read pdftoppm output: %w", err)
	}
	return data, nil
}

// renderMutool draws page to a temp PNG (mudraw cannot emit JPEG) then re-encodes
// it to JPEG through the shared thumb encoder for a consistent output format.
func (h *Handler) renderMutool(ctx context.Context, path string, page int) ([]byte, error) {
	outPath := fmt.Sprintf("%s.p%d.png", path, page)
	defer func() { _ = os.Remove(outPath) }()
	if _, err := mediaproc.Run(ctx, thumbTimeout, h.bin, "draw", "-q",
		"-o", outPath, "-w", strconv.Itoa(thumbMaxDim), path, strconv.Itoa(page)); err != nil {
		return nil, err
	}
	return pngToJPEG(outPath)
}

// pngToJPEG decodes a PNG file and re-encodes it as a JPEG thumbnail.
func pngToJPEG(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read mutool output: %w", err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode mutool png: %w", err)
	}
	var buf bytes.Buffer
	if err := thumb.Encode(img, &buf, thumbMaxDim); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// countPages returns the number of pages in the PDF at path. pdftoppm builds use
// pdfinfo (poppler); mutool builds parse `mutool info`. Both emit a "Pages: N"
// line, so a shared parser extracts the count. A missing pdfinfo (or no renderer)
// yields domain.ErrNoThumbnail so the indexer skips paging.
func (h *Handler) countPages(ctx context.Context, path string) (int, error) {
	switch h.renderer {
	case rendererPdftoppm:
		if h.infoBin == "" {
			return 0, domain.ErrNoThumbnail
		}
		out, err := mediaproc.Run(ctx, probeTimeout, h.infoBin, path)
		if err != nil {
			return 0, fmt.Errorf("pdfinfo: %w", err)
		}
		return parsePagesLine(out)
	case rendererMutool:
		// `mutool info` prints document metadata including a "Pages: N" line.
		out, err := mediaproc.Run(ctx, probeTimeout, h.bin, "info", path)
		if err != nil {
			return 0, fmt.Errorf("mutool info: %w", err)
		}
		return parsePagesLine(out)
	case rendererNone:
		return 0, domain.ErrNoThumbnail
	}
	return 0, domain.ErrNoThumbnail
}

// parsePagesLine extracts the integer from the first line whose trimmed text
// starts with "Pages:" (both pdfinfo and `mutool info` emit this). It is pure so
// it is unit-testable without the tools installed.
func parsePagesLine(out []byte) (int, error) {
	for _, line := range strings.Split(string(out), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Pages:")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(rest))
		if err != nil {
			return 0, fmt.Errorf("parse pages %q: %w", strings.TrimSpace(rest), err)
		}
		return n, nil
	}
	return 0, fmt.Errorf("no Pages line: %w", domain.ErrNoThumbnail)
}
