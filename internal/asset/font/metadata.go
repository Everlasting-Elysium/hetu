package font

import (
	"context"
	"io"
	"strings"

	"golang.org/x/image/font/sfnt"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

var _ kernel.MetadataExtractor = (*Handler)(nil)

// ExtractMetadata reads the font's family and subfamily (style) names from the
// OpenType name table and derives a weight label from the subfamily. It is
// best-effort: an unparseable font (or a missing name) yields whatever was read
// and a nil error so a scan never fails. Implements kernel.MetadataExtractor.
func (h *Handler) ExtractMetadata(_ context.Context, src io.ReadSeeker) (domain.ExtractedMetadata, error) {
	md := domain.ExtractedMetadata{Annotations: make(map[string]any)}
	f, err := parse(src)
	if err != nil {
		return md, nil
	}
	var buf sfnt.Buffer
	if family := readName(f, &buf, sfnt.NameIDTypographicFamily, sfnt.NameIDFamily); family != "" {
		md.Annotations[domain.KeyFontFamily] = family
	}
	if style := readName(f, &buf, sfnt.NameIDTypographicSubfamily, sfnt.NameIDSubfamily); style != "" {
		md.Annotations[domain.KeyFontStyle] = style
		md.Annotations[domain.KeyFontWeight] = weightFromStyle(style)
	}
	return md, nil
}

// readName returns the first non-empty name among ids (typographic name
// preferred over the legacy one), or "" when none is present.
func readName(f *sfnt.Font, buf *sfnt.Buffer, ids ...sfnt.NameID) string {
	for _, id := range ids {
		if s, err := f.Name(buf, id); err == nil {
			if s = strings.TrimSpace(s); s != "" {
				return s
			}
		}
	}
	return ""
}

// weightWords maps a lowercase weight keyword to its canonical label. Compound
// keywords precede their substrings ("semibold" before "bold", "extralight"
// before "light") so the most specific match wins.
var weightWords = []struct{ key, label string }{
	{"extralight", "ExtraLight"},
	{"ultralight", "UltraLight"},
	{"semibold", "SemiBold"},
	{"extrabold", "ExtraBold"},
	{"thin", "Thin"},
	{"light", "Light"},
	{"medium", "Medium"},
	{"black", "Black"},
	{"heavy", "Heavy"},
	{"bold", "Bold"},
}

// weightFromStyle derives a weight label from a subfamily/style string (e.g.
// "Bold Italic" → "Bold"). It defaults to "Regular" when no weight keyword is
// present, since a subfamily always names at least the regular weight.
func weightFromStyle(style string) string {
	s := strings.ToLower(style)
	for _, w := range weightWords {
		if strings.Contains(s, w.key) {
			return w.label
		}
	}
	return "Regular"
}
