package image

import (
	"bytes"
	"context"
	"fmt"
	stdimage "image"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/disintegration/imaging"

	"github.com/Everlasting-Elysium/hetu/internal/asset/mediaproc"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// sniffLen is how many leading bytes decode reads to classify the container.
const sniffLen = 16

// Extract returns kind=image plus dimensions read cheaply from embedded EXIF
// (RAW/HEIC are TIFF/EXIF-based). It deliberately avoids a full external decode —
// that happens once in Thumbnail — so scanning stays fast even for reconnected
// files. Absent dimensions yield zero + nil error, matching the video handler.
func (h *ProHandler) Extract(_ context.Context, src io.ReadSeeker) (domain.Meta, error) {
	w, ht := exifDimensions(src)
	return domain.Meta{Kind: domain.KindImage, Width: w, Height: ht}, nil
}

// Thumbnail decodes src to an sRGB raster via the format's external converter,
// then fits and JPEG-encodes it through the shared pipeline. It returns
// domain.ErrNoThumbnail when no converter is available or every candidate fails,
// so the indexer records the asset without a thumbnail instead of failing.
func (h *ProHandler) Thumbnail(ctx context.Context, src io.ReadSeeker, w io.Writer) error {
	img, err := h.decode(ctx, src)
	if err != nil {
		return domain.ErrNoThumbnail
	}
	return encodeThumbnail(img, w)
}

// decode converts a professional image to a standard raster. It sniffs the
// container, then tries each resolved converter for that format until one yields
// a decodable image, returning the last error if all fail.
func (h *ProHandler) decode(ctx context.Context, src io.ReadSeeker) (stdimage.Image, error) {
	head := make([]byte, sniffLen)
	n, _ := io.ReadFull(src, head)
	format := sniffProFormat(head[:n])

	tools := h.tools[format]
	if len(tools) == 0 {
		h.warnMissing(ctx)
		return nil, domain.ErrNoThumbnail
	}
	inPath, cleanup, err := mediaproc.TempCopy(src, proInputSuffix(format))
	if err != nil {
		return nil, err
	}
	defer cleanup()

	timeout := formatTimeout(format)
	var lastErr error
	for _, tool := range tools {
		img, err := runConverter(ctx, tool, inPath, timeout)
		if err == nil {
			return img, nil
		}
		lastErr = err
		h.log.DebugContext(ctx, "pro converter failed; trying next",
			slog.String("bin", tool.bin), slog.Any("err", err))
	}
	return nil, lastErr
}

// runConverter executes one tool and decodes its output, from stdout for
// streaming tools or from a scratch temp file for file-only tools (mirroring the
// pdftoppm-vs-mutool split in the document handler). All temp state is cleaned up.
func runConverter(ctx context.Context, tool proTool, inPath string, timeout time.Duration) (stdimage.Image, error) {
	if tool.stdout {
		out, err := mediaproc.Run(ctx, timeout, tool.bin, tool.args(inPath, "", "")...)
		if err != nil {
			return nil, err
		}
		return imaging.Decode(bytes.NewReader(out))
	}
	workDir, err := os.MkdirTemp("", "hetu-pro-*")
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()
	outPath := filepath.Join(workDir, "out"+tool.outExt)
	if _, err := mediaproc.Run(ctx, timeout, tool.bin, tool.args(inPath, workDir, outPath)...); err != nil {
		return nil, err
	}
	return imaging.Open(outPath)
}

func formatTimeout(f proFormat) time.Duration {
	switch f {
	case fmtRAW:
		return rawTimeout
	case fmtHEIC:
		return heicTimeout
	default:
		return exrTimeout
	}
}

// proInputSuffix names the temp input file so extension-sensitive tools
// (oiiotool, ImageMagick) pick the right reader; content-based tools ignore it.
func proInputSuffix(f proFormat) string {
	switch f {
	case fmtEXR:
		return ".exr"
	case fmtHEIC:
		return ".heic"
	default:
		return ".dng"
	}
}
