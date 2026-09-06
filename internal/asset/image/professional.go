// This file extends the image package with a decode frontend for professional
// formats — camera RAW, HEIC/HEIF, and OpenEXR — that the pure-Go decoders
// cannot read. Following the video/audio handlers, it shells out to external
// converters (dcraw, darktable-cli, libheif, oiiotool, ffmpeg, ImageMagick),
// keeping the kernel CGO-free. Each tool converts the source to an sRGB raster
// which is then fed through the SAME thumbnail pipeline (see render.go). When no
// converter is present the handler degrades gracefully: assets are still indexed
// as kind=image, just without dimensions or a thumbnail.
package image

import (
	"context"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// proFormat classifies a professional image by container so the right external
// converter is chosen. Detection is by magic bytes (see sniffProFormat), not by
// extension, because AssetHandler methods receive only a reader.
type proFormat int

const (
	fmtRAW proFormat = iota
	fmtHEIC
	fmtEXR
)

// Per-format subprocess timeouts. RAW demosaicing on a large sensor file is far
// slower than decoding a single HEIC or EXR frame.
const (
	rawTimeout  = 120 * time.Second
	heicTimeout = 60 * time.Second
	exrTimeout  = 60 * time.Second
)

// proSupported maps professional extensions to their container class. None of
// these overlap the pure-Go handler's set (jpg/png/gif/bmp/tiff), so both
// handlers coexist under kind=image with unambiguous extension routing.
var proSupported = map[string]proFormat{
	// Camera RAW (TIFF-based and maker-specific containers).
	"cr2": fmtRAW, "cr3": fmtRAW, "nef": fmtRAW, "nrw": fmtRAW, "arw": fmtRAW,
	"srf": fmtRAW, "sr2": fmtRAW, "raf": fmtRAW, "orf": fmtRAW, "rw2": fmtRAW,
	"pef": fmtRAW, "dng": fmtRAW, "dcr": fmtRAW, "kdc": fmtRAW, "mrw": fmtRAW,
	"x3f": fmtRAW, "3fr": fmtRAW, "mef": fmtRAW, "iiq": fmtRAW, "rwl": fmtRAW,
	"srw": fmtRAW,
	// HEIC / HEIF (ISO-BMFF, HEVC-coded stills).
	"heic": fmtHEIC, "heif": fmtHEIC, "hif": fmtHEIC,
	// OpenEXR (scene-linear HDR).
	"exr": fmtEXR,
}

// ProHandler decodes professional image formats via external converters. It
// implements AssetHandler and MetadataExtractor, but deliberately NOT
// PaletteExtractor/PHashExtractor: the indexer opens a fresh reader per
// capability, so implementing those would trigger a second and third expensive
// external decode of the same (often 50-100MB) file per scan.
type ProHandler struct {
	tools map[proFormat][]proTool // resolved & available, in preference order
	log   *slog.Logger
	warn  sync.Once
}

var (
	_ kernel.AssetHandler      = (*ProHandler)(nil)
	_ kernel.MetadataExtractor = (*ProHandler)(nil)
)

// NewPro returns a professional-image handler, resolving each format's available
// external converters on PATH once. A nil log falls back to slog.Default.
func NewPro(log *slog.Logger) *ProHandler {
	if log == nil {
		log = slog.Default()
	}
	tools := make(map[proFormat][]proTool)
	for format, candidates := range proToolsByFormat {
		for _, t := range candidates {
			if path, err := exec.LookPath(t.bin); err == nil {
				t.bin = path
				tools[format] = append(tools[format], t)
			}
		}
	}
	return &ProHandler{tools: tools, log: log}
}

// Match reports whether ext is a supported professional image extension.
func (h *ProHandler) Match(ext string) bool {
	_, ok := proSupported[ext]
	return ok
}

// Kind returns domain.KindImage: professional formats are still images.
func (h *ProHandler) Kind() domain.AssetKind { return domain.KindImage }

func (h *ProHandler) warnMissing(ctx context.Context) {
	h.warn.Do(func() {
		h.log.WarnContext(ctx,
			"no external converter on PATH for a professional image format; "+
				"affected RAW/HEIC/EXR files are indexed without a thumbnail",
			slog.String("hint", "install dcraw/darktable-cli, libheif (heif-dec), or oiiotool/ffmpeg/imagemagick"))
	})
}

// sniffProFormat classifies the leading bytes of a file by magic number. EXR and
// the ISO-BMFF "ftyp" box (HEIC/HEIF) are unambiguous; everything else is
// treated as RAW (TIFF-based and maker containers).
func sniffProFormat(head []byte) proFormat {
	if len(head) >= 4 && head[0] == 0x76 && head[1] == 0x2F && head[2] == 0x31 && head[3] == 0x01 {
		return fmtEXR
	}
	if len(head) >= 8 && string(head[4:8]) == "ftyp" {
		return fmtHEIC
	}
	return fmtRAW
}
