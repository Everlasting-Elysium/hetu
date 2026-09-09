// Package font implements kernel.AssetHandler for OpenType/TrueType font files.
// It parses the font with the pure-Go golang.org/x/image/font/opentype decoder
// and renders a specimen sheet (sample glyphs set in the font itself) as a JPEG
// thumbnail, keeping the kernel CGO-free. Fonts have no intrinsic raster
// dimensions, so Extract reports only kind=font; when a file cannot be parsed
// (e.g. a WOFF container the decoder does not read) it degrades gracefully — the
// asset is still indexed as kind=font, just without a preview or metadata.
package font

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/Everlasting-Elysium/hetu/internal/asset/thumb"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

const (
	canvasDim    = 512 // width/height (px) of the specimen sheet
	specimenSize = 44  // rendered glyph size (px) for the sample lines
	specimenDPI  = 72  // render 1:1 (size is already in px)
	lineStep     = specimenSize + 40
	leftMargin   = 40
)

var supported = map[string]struct{}{"ttf": {}, "otf": {}, "woff": {}}

// Handler processes font files. It holds no state.
type Handler struct{}

var _ kernel.AssetHandler = (*Handler)(nil)

// New returns a font handler.
func New() *Handler { return &Handler{} }

// Match reports whether ext is a supported font extension. woff2 is excluded:
// the opentype decoder cannot read its Brotli-compressed container.
func (h *Handler) Match(ext string) bool {
	_, ok := supported[ext]
	return ok
}

// Kind returns domain.KindFont.
func (h *Handler) Kind() domain.AssetKind { return domain.KindFont }

// Extract returns kind=font. Fonts have no raster dimensions, and parsing is
// deferred to Thumbnail/ExtractMetadata so a scan never fails on a font file.
func (h *Handler) Extract(_ context.Context, _ io.ReadSeeker) (domain.Meta, error) {
	return domain.Meta{Kind: domain.KindFont}, nil
}

// Thumbnail renders a specimen sheet set in the font itself and writes it as
// JPEG into w, or returns domain.ErrNoThumbnail when the font cannot be parsed
// or a face cannot be built.
func (h *Handler) Thumbnail(_ context.Context, src io.ReadSeeker, w io.Writer) error {
	f, err := parse(src)
	if err != nil {
		return domain.ErrNoThumbnail
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size: specimenSize, DPI: specimenDPI, Hinting: font.HintingFull,
	})
	if err != nil {
		return domain.ErrNoThumbnail
	}
	defer func() { _ = face.Close() }()
	if err := thumb.Encode(renderSpecimen(f, face), w, canvasDim); err != nil {
		return fmt.Errorf("encode font specimen: %w", err)
	}
	return nil
}

// renderSpecimen draws sample lines onto a white canvas with the given face. It
// always includes Latin letters and digits; a third CJK line is added only when
// the font actually contains those glyphs (no fallback font is substituted).
func renderSpecimen(f *sfnt.Font, face font.Face) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, canvasDim, canvasDim))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	lines := []string{"AaBbCcDd", "0123456789"}
	if hasGlyph(f, '你') {
		lines = append(lines, "你好世界 Hello")
	}
	drawer := &font.Drawer{Dst: canvas, Src: image.NewUniform(color.Black), Face: face}
	y := lineStep
	for _, line := range lines {
		drawer.Dot = fixed.P(leftMargin, y)
		drawer.DrawString(line)
		y += lineStep
	}
	return canvas
}

// hasGlyph reports whether the font contains a glyph for r.
func hasGlyph(f *sfnt.Font, r rune) bool {
	var buf sfnt.Buffer
	idx, err := f.GlyphIndex(&buf, r)
	return err == nil && idx != 0
}

// parse reads the whole font file and decodes it with opentype.Parse.
func parse(src io.ReadSeeker) (*sfnt.Font, error) {
	data, err := io.ReadAll(src)
	if err != nil {
		return nil, fmt.Errorf("read font: %w", err)
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse font: %w", err)
	}
	return f, nil
}
