package psd

import (
	"context"
	"io"

	"github.com/oov/psd"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

var _ kernel.MetadataExtractor = (*Handler)(nil)

// ExtractMetadata reads the PSD's layer count, color mode, and bit depth into
// extracted-layer annotations. It decodes structure only (both the merged image
// and per-layer image data are skipped) so it stays cheap on large files. It is
// best-effort: a decode failure yields no annotations and a nil error so a scan
// never fails on a malformed PSD. Implements kernel.MetadataExtractor.
func (h *Handler) ExtractMetadata(_ context.Context, src io.ReadSeeker) (domain.ExtractedMetadata, error) {
	md := domain.ExtractedMetadata{Annotations: make(map[string]any)}
	img, _, err := psd.Decode(src, &psd.DecodeOptions{SkipMergedImage: true, SkipLayerImage: true})
	if err != nil {
		return md, nil
	}
	md.Annotations[domain.KeyPSDLayerCount] = countLayers(img.Layer, 0)
	if mode := colorModeName(img.Config.ColorMode); mode != "" {
		md.Annotations[domain.KeyPSDColorMode] = mode
	}
	if img.Config.Depth > 0 {
		md.Annotations[domain.KeyPSDBitDepth] = img.Config.Depth
	}
	return md, nil
}

// maxLayerDepth bounds group-nesting recursion so a hostile PSD with absurdly
// deep group nesting cannot exhaust the stack (a stack overflow is fatal and not
// recoverable). Real documents never approach this depth.
const maxLayerDepth = 100

// countLayers totals every layer, descending into group folders (bounded by
// maxLayerDepth) so a grouped document reports its full layer count rather than
// just its top-level entries.
func countLayers(layers []psd.Layer, depth int) int {
	if depth >= maxLayerDepth {
		return 0
	}
	n := len(layers)
	for i := range layers {
		n += countLayers(layers[i].Layer, depth+1)
	}
	return n
}

// colorModeName maps a psd.ColorMode to a human label, or "" for an unknown
// mode (skipped rather than stored as a bare number).
func colorModeName(m psd.ColorMode) string {
	switch m {
	case psd.ColorModeBitmap:
		return "Bitmap"
	case psd.ColorModeGrayscale:
		return "Grayscale"
	case psd.ColorModeIndexed:
		return "Indexed"
	case psd.ColorModeRGB:
		return "RGB"
	case psd.ColorModeCMYK:
		return "CMYK"
	case psd.ColorModeMultichannel:
		return "Multichannel"
	case psd.ColorModeDuotone:
		return "Duotone"
	case psd.ColorModeLab:
		return "Lab"
	}
	return ""
}
