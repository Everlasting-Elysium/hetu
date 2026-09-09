// Package psd implements kernel.AssetHandler for Adobe Photoshop documents. It
// reads dimensions from the file header and renders the embedded merged
// (composite) image to a JPEG thumbnail with the pure-Go oov/psd decoder,
// keeping the kernel CGO-free. hetu never composites layers itself (Photoshop's
// blend modes are out of scope), so a PSD saved without a merged image simply
// has no thumbnail. A flattened PSD is a raster image, so it is indexed as
// kind=image — reusing every image filter/board/palette path downstream —
// rather than introducing a PSD-specific kind.
package psd

import (
	"context"
	"fmt"
	"io"

	"github.com/oov/psd"

	"github.com/Everlasting-Elysium/hetu/internal/asset/thumb"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// thumbMaxDim is the longest edge (px) of a generated thumbnail; it matches the
// image handler so PSD and raster thumbnails are the same size.
const thumbMaxDim = 512

// Handler processes Adobe Photoshop (.psd) documents. Like the pure-Go image
// handler it holds no state.
type Handler struct{}

var _ kernel.AssetHandler = (*Handler)(nil)

// New returns a PSD handler.
func New() *Handler { return &Handler{} }

// Match reports whether ext is a supported Photoshop extension.
func (h *Handler) Match(ext string) bool { return ext == "psd" }

// Kind returns domain.KindImage: a flattened PSD is a raster image, so it reuses
// the image kind rather than introducing a PSD-specific one.
func (h *Handler) Kind() domain.AssetKind { return domain.KindImage }

// Extract reads the PSD header for dimensions. It is best-effort: a malformed
// header yields kind=image with zero dimensions and a nil error so a scan still
// indexes the file (matching the video/audio degradation contract).
func (h *Handler) Extract(_ context.Context, src io.ReadSeeker) (domain.Meta, error) {
	cfg, _, err := psd.DecodeConfig(src)
	if err != nil {
		return domain.Meta{Kind: domain.KindImage}, nil
	}
	return domain.Meta{Kind: domain.KindImage, Width: cfg.Rect.Dx(), Height: cfg.Rect.Dy()}, nil
}

// Thumbnail decodes the PSD's merged (composite) image and writes it as a JPEG
// thumbnail into w, or returns domain.ErrNoThumbnail when the file cannot be
// decoded or carries no merged image. Per-layer image data is skipped: only the
// composite is needed, and hetu never composites layers itself.
func (h *Handler) Thumbnail(_ context.Context, src io.ReadSeeker, w io.Writer) error {
	// Read the header first and reject an image whose declared dimensions would
	// force a huge allocation in psd.Decode (which sizes its pixel buffers from
	// the header). DecodeConfig only reads the header, so this stays cheap.
	cfg, _, err := psd.DecodeConfig(src)
	if err != nil {
		return domain.ErrNoThumbnail
	}
	if !thumb.WithinLimits(cfg.Rect.Dx(), cfg.Rect.Dy()) {
		return domain.ErrNoThumbnail
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek psd source: %w", err)
	}
	img, _, err := psd.Decode(src, &psd.DecodeOptions{SkipLayerImage: true})
	if err != nil {
		return domain.ErrNoThumbnail
	}
	if img.Picker == nil {
		return domain.ErrNoThumbnail
	}
	if err := thumb.Encode(img.Picker, w, thumbMaxDim); err != nil {
		return fmt.Errorf("encode psd thumbnail: %w", err)
	}
	return nil
}
