package dam

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// maxVersionUpload caps an uploaded version body (256 MiB) to bound memory/disk.
const maxVersionUpload = 256 << 20

// uploadVersion adds a new revision of an asset and makes it current.
// POST /api/dam/assets/{id}/versions  (multipart/form-data: file=<bytes>, note=<text>)
// The bytes are copied into the managed tree (never in place); the asset's
// anchor storage_path/hash stay put, so scan/dedup/relocate are unaffected.
func (p *Plugin) uploadVersion(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	asset, err := p.k.Store.GetAsset(r.Context(), p.owner, id)
	if err != nil {
		writeAssetLookupError(w, err)
		return
	}
	// A trashed asset is on its way out (PurgeTrash would orphan the version
	// bytes); require restoring it before versioning.
	if asset.DeletedAt != nil {
		httpjson.WriteError(w, http.StatusConflict,
			errors.New("cannot add a version to a trashed asset; restore it first"))
		return
	}

	writer, ok := p.storageWriter(asset.Provider)
	if !ok {
		httpjson.WriteError(w, http.StatusBadRequest,
			fmt.Errorf("provider %q does not support versioning: %w", asset.Provider, domain.ErrUnsupported))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxVersionUpload)
	file, header, err := r.FormFile("file")
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("read upload file: %w", err))
		return
	}
	defer func() { _ = file.Close() }()
	note := r.FormValue("note")

	created, err := p.storeVersion(r.Context(), asset, file, header.Filename, note, writer)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toVersionDTO(created, true))
}

// storeVersion writes the uploaded bytes into the managed tree, derives
// hash/dims/thumbnail, then commits the version (with lazy v1 backfill) as
// current. On DB failure it removes the copied file and thumbnail so a committed
// row never points at missing bytes.
func (p *Plugin) storeVersion(ctx context.Context, asset domain.Asset, file io.ReadSeeker, filename, note string, writer kernel.StorageWriter) (domain.AssetVersion, error) {
	versionID, err := newVersionID()
	if err != nil {
		return domain.AssetVersion{}, err
	}
	name := versionFilename(filename)
	storagePath := domain.VersionStoragePath(asset.ID, versionID, name)

	hash, size, err := hashAndSize(file)
	if err != nil {
		return domain.AssetVersion{}, fmt.Errorf("hash upload: %w", err)
	}
	width, height, thumbPath := p.deriveVersionMedia(ctx, file, name, versionID.String())

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		p.cleanupVersionThumb(thumbPath)
		return domain.AssetVersion{}, fmt.Errorf("rewind upload: %w", err)
	}
	if _, err := writer.Write(ctx, storagePath, file); err != nil {
		p.cleanupVersionThumb(thumbPath)
		return domain.AssetVersion{}, fmt.Errorf("write version bytes: %w", err)
	}

	newVersion := domain.AssetVersion{
		ID: versionID, AssetID: asset.ID, Owner: p.owner,
		Provider: asset.Provider, StoragePath: storagePath, Hash: hash, Size: size,
		ThumbPath: thumbPath, Width: width, Height: height, Note: note,
		CreatedAt: time.Now().UTC(),
	}
	baseID, err := newVersionID()
	if err != nil {
		_ = writer.Remove(ctx, storagePath)
		p.cleanupVersionThumb(thumbPath)
		return domain.AssetVersion{}, err
	}
	created, err := p.k.Store.AddVersion(ctx, p.owner, p.anchorVersion(asset, baseID), newVersion)
	if err != nil {
		_ = writer.Remove(ctx, storagePath)
		p.cleanupVersionThumb(thumbPath)
		return domain.AssetVersion{}, fmt.Errorf("add version: %w", err)
	}
	return created, nil
}

// anchorVersion builds the version-1 candidate from the asset's anchor state.
// AddVersion uses it only when the asset has no explicit versions yet; its
// storage_path is the original in-place file (outside the managed tree).
func (p *Plugin) anchorVersion(a domain.Asset, id domain.VersionID) domain.AssetVersion {
	return domain.AssetVersion{
		ID: id, AssetID: a.ID, Owner: p.owner, Provider: a.Provider,
		StoragePath: a.StoragePath, Hash: a.Hash, Size: a.Size,
		ThumbPath: a.ThumbPath, Width: a.Width, Height: a.Height,
		Note: "initial", CreatedAt: a.CreatedAt,
	}
}

// deriveVersionMedia extracts dimensions and renders a thumbnail from the
// uploaded bytes using the handler for the file's extension. It is best-effort:
// an unsupported type or a failed decode yields zero dims and no thumbnail
// rather than failing the upload (the version bytes are still stored).
func (p *Plugin) deriveVersionMedia(ctx context.Context, file io.ReadSeeker, name, versionID string) (int, int, string) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	handler, ok := p.k.Assets.HandlerFor(ext)
	if !ok {
		return 0, 0, ""
	}
	var width, height int
	if _, err := file.Seek(0, io.SeekStart); err == nil {
		if meta, err := handler.Extract(ctx, file); err == nil {
			width, height = meta.Width, meta.Height
		}
	}
	thumbPath := ""
	if _, err := file.Seek(0, io.SeekStart); err == nil {
		if tp, err := p.versionThumb(ctx, file, handler, versionID); err != nil {
			p.k.Log.WarnContext(ctx, "version thumbnail", slog.Any("err", err))
		} else {
			thumbPath = tp
		}
	}
	return width, height, thumbPath
}

// versionThumb renders a JPEG thumbnail into ThumbDir keyed by version id (never
// the asset id, so it never collides with the anchor's own thumbnail). Returns
// an empty path when the handler cannot produce one (domain.ErrNoThumbnail).
func (p *Plugin) versionThumb(ctx context.Context, src io.Reader, h kernel.AssetHandler, versionID string) (string, error) {
	if err := os.MkdirAll(p.k.ThumbDir, 0o755); err != nil {
		return "", fmt.Errorf("create thumb dir: %w", err)
	}
	thumbPath := filepath.Join(p.k.ThumbDir, versionID+".jpg")
	out, err := os.Create(thumbPath)
	if err != nil {
		return "", fmt.Errorf("create thumb: %w", err)
	}
	rs, ok := src.(io.ReadSeeker)
	if !ok {
		_ = out.Close()
		_ = os.Remove(thumbPath)
		return "", errors.New("thumbnail source not seekable")
	}
	genErr := h.Thumbnail(ctx, rs, out)
	closeErr := out.Close()
	if genErr != nil {
		_ = os.Remove(thumbPath)
		if errors.Is(genErr, domain.ErrNoThumbnail) {
			return "", nil
		}
		return "", fmt.Errorf("thumbnail: %w", genErr)
	}
	if closeErr != nil {
		_ = os.Remove(thumbPath)
		return "", fmt.Errorf("close thumb: %w", closeErr)
	}
	return thumbPath, nil
}

// hashAndSize computes the SHA-256 and byte length of rs from its start.
func hashAndSize(rs io.ReadSeeker) (string, int64, error) {
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	sum := sha256.New()
	n, err := io.Copy(sum, rs)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(sum.Sum(nil)), n, nil
}
