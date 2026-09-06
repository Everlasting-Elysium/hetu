package dam

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
)

// listVersions returns an asset's revision history, newest first, with the
// current version flagged. GET /api/dam/assets/{id}/versions. An asset with no
// explicit versions yet returns an empty list (its single file is the anchor).
func (p *Plugin) listVersions(w http.ResponseWriter, r *http.Request) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}
	currentID, err := p.k.Store.CurrentVersionID(r.Context(), p.owner, id)
	if err != nil {
		writeAssetLookupError(w, err)
		return
	}
	versions, err := p.k.Store.ListVersions(r.Context(), p.owner, id)
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	dtos := make([]versionDTO, 0, len(versions))
	for _, v := range versions {
		dtos = append(dtos, toVersionDTO(v, v.ID.String() == currentID))
	}
	httpjson.WriteJSON(w, http.StatusOK, dtos)
}

// setCurrentVersion makes an existing version the current one (rollback/switch).
// POST /api/dam/assets/{id}/versions/{no}/current
func (p *Plugin) setCurrentVersion(w http.ResponseWriter, r *http.Request) {
	id, no, ok := p.assetAndVersionNo(w, r)
	if !ok {
		return
	}
	version, err := p.k.Store.GetVersionByNo(r.Context(), p.owner, id, no)
	if err != nil {
		writeAssetLookupError(w, err)
		return
	}
	// SetCurrentVersion re-verifies existence in its transaction, so a version
	// deleted between the lookup above and here maps to 404, not a dangling
	// pointer.
	if err := p.k.Store.SetCurrentVersion(r.Context(), p.owner, id, version.ID); err != nil {
		writeAssetLookupError(w, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"current": no})
}

// deleteVersion removes an old version. The current version cannot be deleted
// (switch first). The physical file is removed only when it lives in the managed
// tree, and the thumbnail only when it is the version's own — so deleting the
// backfilled version 1 never touches the user's in-place original or its shared
// thumbnail. DELETE /api/dam/assets/{id}/versions/{no}
func (p *Plugin) deleteVersion(w http.ResponseWriter, r *http.Request) {
	id, no, ok := p.assetAndVersionNo(w, r)
	if !ok {
		return
	}
	// Resolve version_no -> version (404) and keep its paths for file cleanup.
	version, err := p.k.Store.GetVersionByNo(r.Context(), p.owner, id, no)
	if err != nil {
		writeAssetLookupError(w, err)
		return
	}
	// DeleteVersion atomically refuses (deleted=false) when the version is
	// current, so there is no racy check-then-delete window here.
	deleted, err := p.k.Store.DeleteVersion(r.Context(), p.owner, id, version.ID)
	if err != nil {
		writeAssetLookupError(w, err)
		return
	}
	if !deleted {
		httpjson.WriteError(w, http.StatusConflict,
			errors.New("cannot delete the current version; set another version current first"))
		return
	}
	p.removeVersionFiles(r.Context(), version)
	httpjson.WriteJSON(w, http.StatusOK, map[string]int{"deleted": no})
}

// removeVersionFiles deletes a deleted version's bytes and thumbnail, guarding
// the in-place original (only managed paths are removed) and the shared anchor
// thumbnail (only the version's own {versionID}.jpg is removed).
func (p *Plugin) removeVersionFiles(ctx context.Context, v domain.AssetVersion) {
	if domain.IsManagedPath(v.StoragePath) {
		if writer, ok := p.storageWriter(v.Provider); ok {
			if err := writer.Remove(ctx, v.StoragePath); err != nil {
				p.k.Log.WarnContext(ctx, "remove version file", slog.Any("err", err))
			}
		}
	}
	ownThumb := filepath.Join(p.k.ThumbDir, v.ID.String()+".jpg")
	if v.ThumbPath == ownThumb {
		_ = os.Remove(v.ThumbPath)
	}
}

// assetAndVersionNo parses the {id} and {no} path params shared by set-current
// and delete, writing a 400 and returning ok=false on failure.
func (p *Plugin) assetAndVersionNo(w http.ResponseWriter, r *http.Request) (domain.AssetID, int, bool) {
	id, err := domain.NewAssetID(chi.URLParam(r, "id"))
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return domain.AssetID{}, 0, false
	}
	raw := chi.URLParam(r, "no")
	no, err := strconv.Atoi(raw)
	if err != nil || no < 1 {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid version number %q", raw))
		return domain.AssetID{}, 0, false
	}
	return id, no, true
}

// storageWriter returns the named provider as a kernel.StorageWriter, or
// ok=false when it is unregistered or read-only (does not support writes).
func (p *Plugin) storageWriter(provider string) (kernel.StorageWriter, bool) {
	sp, ok := p.k.Storage.Get(provider)
	if !ok {
		return nil, false
	}
	writer, ok := sp.(kernel.StorageWriter)
	return writer, ok
}

// cleanupVersionThumb removes a partially-written version thumbnail on failure.
func (p *Plugin) cleanupVersionThumb(thumbPath string) {
	if thumbPath != "" {
		_ = os.Remove(thumbPath)
	}
}

// newVersionID mints a UUIDv7 VersionID.
func newVersionID() (domain.VersionID, error) {
	u, err := uuid.NewV7()
	if err != nil {
		return domain.VersionID{}, fmt.Errorf("generate version id: %w", err)
	}
	return domain.NewVersionID(u.String())
}

// versionFilename sanitizes an uploaded filename to a single path segment.
func versionFilename(name string) string {
	base := filepath.Base(name)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "version"
	}
	return base
}

// writeAssetLookupError maps a store lookup error to 404 (ErrNotFound) or 500.
func writeAssetLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		httpjson.WriteError(w, http.StatusNotFound, err)
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, err)
}

type versionDTO struct {
	ID        string `json:"id"`
	VersionNo int    `json:"version_no"`
	Hash      string `json:"hash"`
	Size      int64  `json:"size"`
	Path      string `json:"path"`
	Thumb     string `json:"thumb"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
	IsCurrent bool   `json:"is_current"`
}

func toVersionDTO(v domain.AssetVersion, isCurrent bool) versionDTO {
	return versionDTO{
		ID:        v.ID.String(),
		VersionNo: v.VersionNo,
		Hash:      v.Hash,
		Size:      v.Size,
		Path:      v.StoragePath,
		Thumb:     v.ThumbPath,
		Width:     v.Width,
		Height:    v.Height,
		Note:      v.Note,
		CreatedAt: v.CreatedAt.Format(time.RFC3339),
		IsCurrent: isCurrent,
	}
}
