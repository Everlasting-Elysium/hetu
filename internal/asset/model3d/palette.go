package model3d

import (
	"bytes"
	"context"
	"fmt"
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

// Palette derives a color palette for a 3D model by rendering it through the
// Blender sidecar (raw geometry carries no pixels), decoding the PNG, and
// quantizing to paletteSize dominant colors. With no sidecar configured it
// returns domain.ErrNoThumbnail so color stays best-effort, exactly like
// Thumbnail. Initial scans render here; a later client-uploaded thumbnail
// re-extracts the palette without Blender (issue #88, see dam.uploadThumb).
func (h *Handler) Palette(ctx context.Context, src io.ReadSeeker) (color.Palette, error) {
	if h.blenderAddr == "" {
		return nil, domain.ErrNoThumbnail
	}
	var buf bytes.Buffer
	if err := renderThumbnail(ctx, h.blenderAddr, src, &buf); err != nil {
		return nil, err
	}
	img, err := imaging.Decode(&buf)
	if err != nil {
		return nil, fmt.Errorf("decode rendered thumbnail: %w", err)
	}
	return color.ExtractPalette(img, sampleMaxDim, paletteSize), nil
}
