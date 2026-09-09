package document

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/asset/mediaproc"
	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

// convertTimeout bounds a LibreOffice conversion: cold starts and large decks
// are slow, but a runaway conversion must not block a scan indefinitely.
const convertTimeout = 120 * time.Second

// convertOffice writes src to a temp file and converts it to PDF with
// LibreOffice headless, returning the produced PDF path plus a cleanup func the
// caller must defer. Each conversion uses a private user-profile dir so
// concurrent conversions do not contend on LibreOffice's global profile lock.
// The produced PDF then flows through the same PDF page pipeline as a native
// PDF, so PPT paging reuses the pdftoppm/mutool code path unchanged.
func (h *Handler) convertOffice(ctx context.Context, src io.ReadSeeker) (string, func(), error) {
	if h.soffice == "" {
		return "", nil, fmt.Errorf("libreoffice not available: %w", domain.ErrNoThumbnail)
	}
	inPath, cleanIn, err := mediaproc.TempCopy(src, ".ppt")
	if err != nil {
		return "", nil, err
	}
	outDir, err := os.MkdirTemp("", "hetu-lo-out-*")
	if err != nil {
		cleanIn()
		return "", nil, fmt.Errorf("create outdir: %w", err)
	}
	userDir, err := os.MkdirTemp("", "hetu-lo-user-*")
	if err != nil {
		cleanIn()
		_ = os.RemoveAll(outDir)
		return "", nil, fmt.Errorf("create user dir: %w", err)
	}
	cleanup := func() {
		cleanIn()
		_ = os.RemoveAll(outDir)
		_ = os.RemoveAll(userDir)
	}
	if _, err := mediaproc.Run(ctx, convertTimeout, h.soffice,
		"--headless", "--nologo", "--nofirststartwizard",
		"-env:UserInstallation=file://"+userDir,
		"--convert-to", "pdf", "--outdir", outDir, inPath); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("libreoffice convert: %w", err)
	}
	// LibreOffice names the output <input-basename>.pdf; glob rather than
	// reconstruct the name so a surprising rename still resolves.
	pdfs, err := filepath.Glob(filepath.Join(outDir, "*.pdf"))
	if err != nil || len(pdfs) == 0 {
		cleanup()
		return "", nil, fmt.Errorf("libreoffice produced no pdf: %w", domain.ErrNoThumbnail)
	}
	return pdfs[0], cleanup, nil
}
