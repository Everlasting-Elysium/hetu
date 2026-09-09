package design

import (
	"archive/zip"
	"bytes"
	"fmt"
	stdimage "image"
	_ "image/jpeg" // register decoders so DecodeConfig/Decode read the sketch preview
	_ "image/png"
	"io"
	"strings"

	"github.com/Everlasting-Elysium/hetu/internal/asset/thumb"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

const (
	// sketchPreviewPath is the canonical location of a Sketch document's preview.
	sketchPreviewPath = "previews/preview.png"
	// previewMaxDim is the longest edge (px) of the generated preview thumbnail.
	previewMaxDim = 512
	// maxSketchBytes caps the .sketch container read so a giant bundle cannot be
	// slurped whole into memory; beyond it, no preview is extracted.
	maxSketchBytes = 128 << 20
	// maxPreviewBytes caps the DECOMPRESSED preview entry read, defusing a zip
	// bomb whose tiny compressed entry would expand to gigabytes.
	maxPreviewBytes = 32 << 20
)

// extractSketchPreview finds the embedded preview image in a Sketch bundle (a
// ZIP archive), fits it through the shared thumb encoder to a 512px JPEG, and
// writes it to w. It returns domain.ErrNoThumbnail when no preview is present,
// the entry is not a decodable image, or its declared dimensions exceed the sane
// limit. Every read is bounded so a hostile bundle cannot exhaust memory.
func extractSketchPreview(src io.ReadSeeker, w io.Writer) error {
	data, err := io.ReadAll(io.LimitReader(src, maxSketchBytes))
	if err != nil {
		return domain.ErrNoThumbnail
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return domain.ErrNoThumbnail
	}
	f := findPreview(zr)
	if f == nil {
		return domain.ErrNoThumbnail
	}
	raw, err := readPreview(f)
	if err != nil {
		return domain.ErrNoThumbnail
	}
	img, err := decodeWithinLimits(raw)
	if err != nil {
		return domain.ErrNoThumbnail
	}
	if err := thumb.Encode(img, w, previewMaxDim); err != nil {
		return fmt.Errorf("encode sketch preview: %w", err)
	}
	return nil
}

// readPreview reads a preview zip entry with a decompressed-size cap so a zip
// bomb cannot expand unbounded.
func readPreview(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(io.LimitReader(rc, maxPreviewBytes))
}

// decodeWithinLimits rejects an image whose declared dimensions exceed the sane
// limit BEFORE allocating its pixel buffer (DecodeConfig reads only the header),
// then decodes it.
func decodeWithinLimits(raw []byte) (stdimage.Image, error) {
	cfg, _, err := stdimage.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if !thumb.WithinLimits(cfg.Width, cfg.Height) {
		return nil, fmt.Errorf("preview dimensions %dx%d exceed limit", cfg.Width, cfg.Height)
	}
	img, _, err := stdimage.Decode(bytes.NewReader(raw))
	return img, err
}

// findPreview returns the canonical previews/preview.png entry when present,
// else the first image under previews/, else nil.
func findPreview(zr *zip.Reader) *zip.File {
	var fallback *zip.File
	for _, f := range zr.File {
		if f.Name == sketchPreviewPath {
			return f
		}
		if fallback == nil && strings.HasPrefix(f.Name, "previews/") && isImageName(f.Name) {
			fallback = f
		}
	}
	return fallback
}

func isImageName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".png") ||
		strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".jpeg")
}
