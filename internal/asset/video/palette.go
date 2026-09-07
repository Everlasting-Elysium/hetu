package video

import (
	"bytes"
	"context"
	"io"

	"github.com/disintegration/imaging"

	"github.com/Everlasting-Elysium/hetu/internal/color"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

const (
	sampleMaxDim = color.DefaultSampleMaxDim
	paletteSize  = color.DefaultPaletteSize
)

var _ kernel.PaletteExtractor = (*Handler)(nil)

// Palette extracts a color palette from one representative keyframe: it decodes
// the same JPEG frame Thumbnail renders, downsamples it, and quantizes to
// paletteSize dominant colors. A missing ffmpeg/ffprobe or any decode failure
// degrades to domain.ErrNoThumbnail, so color is best-effort (issue #88).
func (h *Handler) Palette(ctx context.Context, src io.ReadSeeker) (color.Palette, error) {
	out, err := h.frame(ctx, src)
	if err != nil {
		return nil, err
	}
	img, err := imaging.Decode(bytes.NewReader(out))
	if err != nil {
		return nil, domain.ErrNoThumbnail
	}
	return color.ExtractPalette(img, sampleMaxDim, paletteSize), nil
}
