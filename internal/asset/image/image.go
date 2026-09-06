// Package image implements kernel.AssetHandler for raster images: it reads
// dimensions and renders JPEG thumbnails with the pure-Go imaging library
// (no CGO). Video (ffmpeg) and 3D (Blender headless) handlers come later.
package image

import (
	"context"
	"fmt"
	stdimage "image"
	"io"

	// Register the standard decoders so DecodeConfig/Decode recognise them.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/disintegration/imaging"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// thumbMaxDim is the longest edge (px) of a generated thumbnail.
const thumbMaxDim = 512

var supported = map[string]struct{}{
	"jpg": {}, "jpeg": {}, "png": {}, "gif": {}, "bmp": {}, "tiff": {},
}

// Handler processes raster images.
type Handler struct{}

var _ kernel.AssetHandler = (*Handler)(nil)

// New returns an image handler.
func New() *Handler { return &Handler{} }

// Match reports whether ext is a supported raster image extension.
func (h *Handler) Match(ext string) bool {
	_, ok := supported[ext]
	return ok
}

// Kind returns domain.KindImage.
func (h *Handler) Kind() domain.AssetKind { return domain.KindImage }

// Extract reads image dimensions from the header without a full decode.
func (h *Handler) Extract(_ context.Context, src io.ReadSeeker) (domain.Meta, error) {
	cfg, _, err := stdimage.DecodeConfig(src)
	if err != nil {
		return domain.Meta{}, fmt.Errorf("decode image config: %w", err)
	}
	return domain.Meta{Kind: domain.KindImage, Width: cfg.Width, Height: cfg.Height}, nil
}

// Thumbnail renders a JPEG thumbnail (longest edge thumbMaxDim) into w.
func (h *Handler) Thumbnail(_ context.Context, src io.ReadSeeker, w io.Writer) error {
	img, err := imaging.Decode(src, imaging.AutoOrientation(true))
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	return encodeThumbnail(img, w)
}
