package dam

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// maxThumbUpload caps a client-uploaded thumbnail body (10 MiB). A rendered 3D
// preview is a small PNG/JPEG; anything larger is rejected before buffering.
const maxThumbUpload = 10 << 20

// uploadThumb stores a client-rendered thumbnail for an asset and repoints the
// asset at it: POST /api/dam/assets/{id}/thumb (multipart/form-data: file=<png|jpeg>).
// The browser renders 3D-model previews itself (via <model-viewer>), so hetu no
// longer needs Blender to thumbnail models (issue #78). The bytes overwrite any
// existing thumbnail for the asset.
func (p *Plugin) uploadThumb(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := p.k.Store.GetAsset(r.Context(), p.owner, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxThumbUpload)
	file, _, err := r.FormFile("file")
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("read upload file: %w", err))
		return
	}
	defer func() { _ = file.Close() }()
	if err := ensureImagePNGorJPEG(file); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}

	thumbPath, err := p.saveThumb(id, file)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if err := p.k.Store.UpdateAssetThumbPath(r.Context(), p.owner, id, thumbPath); err != nil {
		_ = os.Remove(thumbPath)
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("update thumb path: %w", err))
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]string{"thumb": thumbPath})
}

// saveThumb writes src to <ThumbDir>/<id>.png, overwriting any existing
// thumbnail. The .png name matches serveThumb's content-type inference.
func (p *Plugin) saveThumb(id domain.AssetID, src io.Reader) (string, error) {
	if err := os.MkdirAll(p.k.ThumbDir, 0o755); err != nil {
		return "", fmt.Errorf("create thumb dir: %w", err)
	}
	thumbPath := filepath.Join(p.k.ThumbDir, id.String()+".png")
	out, err := os.Create(thumbPath)
	if err != nil {
		return "", fmt.Errorf("create thumb file: %w", err)
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		_ = os.Remove(thumbPath)
		return "", fmt.Errorf("write thumb file: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(thumbPath)
		return "", fmt.Errorf("close thumb file: %w", err)
	}
	return thumbPath, nil
}

// ensureImagePNGorJPEG rejects a non-PNG/JPEG upload by sniffing the leading
// bytes (magic numbers), then rewinds src so the full body can be written.
func ensureImagePNGorJPEG(src io.ReadSeeker) error {
	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read upload header: %w", err)
	}
	if ct := http.DetectContentType(head[:n]); ct != "image/png" && ct != "image/jpeg" {
		return fmt.Errorf("unsupported thumbnail type %q: want image/png or image/jpeg", ct)
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind upload: %w", err)
	}
	return nil
}
