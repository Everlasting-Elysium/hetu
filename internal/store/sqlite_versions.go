package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// AddVersion appends newVersion as a revision of its asset and makes it current,
// all in one transaction. When the asset has no explicit versions yet
// (current_version_id == ''), it first synthesizes base as version 1 from the
// asset's anchor state (lazy backfill), so the original in-place file is never
// lost from the history. Returns newVersion with its allocated version_no.
func (s *SQLite) AddVersion(ctx context.Context, owner domain.OwnerID, base, newVersion domain.AssetVersion) (domain.AssetVersion, error) {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return domain.AssetVersion{}, fmt.Errorf("begin add version tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	assetID := newVersion.AssetID.String()
	cur, err := qtx.GetAssetCurrentVersion(ctx, db.GetAssetCurrentVersionParams{
		ID: assetID, OwnerID: owner.String(),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AssetVersion{}, fmt.Errorf("add version: asset %s: %w", newVersion.AssetID, domain.ErrNotFound)
		}
		return domain.AssetVersion{}, fmt.Errorf("add version: read current: %w", err)
	}

	if cur == "" {
		// Lazy backfill: the anchor becomes version 1, the upload version 2.
		base.VersionNo = 1
		if err := createVersionTx(ctx, qtx, base); err != nil {
			return domain.AssetVersion{}, err
		}
		newVersion.VersionNo = 2
	} else {
		maxNo, err := qtx.MaxVersionNo(ctx, assetID)
		if err != nil {
			return domain.AssetVersion{}, fmt.Errorf("add version: max no: %w", err)
		}
		newVersion.VersionNo = int(maxNo) + 1
	}

	if err := createVersionTx(ctx, qtx, newVersion); err != nil {
		return domain.AssetVersion{}, err
	}
	if err := qtx.SetAssetCurrentVersion(ctx, db.SetAssetCurrentVersionParams{
		CurrentVersionID: newVersion.ID.String(),
		ID:               assetID,
		OwnerID:          owner.String(),
	}); err != nil {
		return domain.AssetVersion{}, fmt.Errorf("add version: set current: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.AssetVersion{}, fmt.Errorf("commit add version: %w", err)
	}
	return newVersion, nil
}

// ListVersions returns all versions of an asset, newest version first.
func (s *SQLite) ListVersions(ctx context.Context, owner domain.OwnerID, assetID domain.AssetID) ([]domain.AssetVersion, error) {
	rows, err := s.q.ListVersions(ctx, db.ListVersionsParams{
		AssetID: assetID.String(), OwnerID: owner.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	out := make([]domain.AssetVersion, 0, len(rows))
	for _, r := range rows {
		v, err := rowToVersion(r)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// GetVersionByNo returns one version by (asset, version number), or ErrNotFound.
func (s *SQLite) GetVersionByNo(ctx context.Context, owner domain.OwnerID, assetID domain.AssetID, versionNo int) (domain.AssetVersion, error) {
	row, err := s.q.GetVersionByNo(ctx, db.GetVersionByNoParams{
		AssetID: assetID.String(), OwnerID: owner.String(), VersionNo: int64(versionNo),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AssetVersion{}, fmt.Errorf("get version %d: %w", versionNo, domain.ErrNotFound)
		}
		return domain.AssetVersion{}, fmt.Errorf("get version %d: %w", versionNo, err)
	}
	return rowToVersion(row)
}

// GetVersionByID returns one version by its id (owner-scoped), or ErrNotFound.
func (s *SQLite) GetVersionByID(ctx context.Context, owner domain.OwnerID, versionID domain.VersionID) (domain.AssetVersion, error) {
	row, err := s.q.GetVersionByID(ctx, db.GetVersionByIDParams{
		ID: versionID.String(), OwnerID: owner.String(),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AssetVersion{}, fmt.Errorf("get version %s: %w", versionID, domain.ErrNotFound)
		}
		return domain.AssetVersion{}, fmt.Errorf("get version %s: %w", versionID, err)
	}
	return rowToVersion(row)
}

// CurrentVersionID returns the asset's current-version pointer ('' when the
// asset has no explicit versions), or ErrNotFound if the asset does not exist.
func (s *SQLite) CurrentVersionID(ctx context.Context, owner domain.OwnerID, assetID domain.AssetID) (string, error) {
	cur, err := s.q.GetAssetCurrentVersion(ctx, db.GetAssetCurrentVersionParams{
		ID: assetID.String(), OwnerID: owner.String(),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("current version: asset %s: %w", assetID, domain.ErrNotFound)
		}
		return "", fmt.Errorf("current version: %w", err)
	}
	return cur, nil
}

// SetCurrentVersion repoints the asset at versionID. The existence probe and the
// pointer update run in one transaction so a concurrent DeleteVersion cannot
// leave current_version_id dangling; returns domain.ErrNotFound if the version
// no longer exists.
func (s *SQLite) SetCurrentVersion(ctx context.Context, owner domain.OwnerID, assetID domain.AssetID, versionID domain.VersionID) error {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set current tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)
	n, err := qtx.CountVersion(ctx, db.CountVersionParams{
		ID: versionID.String(), AssetID: assetID.String(), OwnerID: owner.String(),
	})
	if err != nil {
		return fmt.Errorf("set current version: probe: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("set current version %s: %w", versionID, domain.ErrNotFound)
	}
	if err := qtx.SetAssetCurrentVersion(ctx, db.SetAssetCurrentVersionParams{
		CurrentVersionID: versionID.String(),
		ID:               assetID.String(),
		OwnerID:          owner.String(),
	}); err != nil {
		return fmt.Errorf("set current version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set current: %w", err)
	}
	return nil
}

// DeleteVersion removes a version row unless it is the asset's current version,
// atomically (read-current + delete in one transaction) so the current pointer
// can never be left dangling by a concurrent set-current. It returns
// deleted=false (no error) when the version is current. The caller removes the
// physical file/thumbnail only when deleted=true (it owns storage access).
func (s *SQLite) DeleteVersion(ctx context.Context, owner domain.OwnerID, assetID domain.AssetID, versionID domain.VersionID) (bool, error) {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin delete version tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)
	cur, err := qtx.GetAssetCurrentVersion(ctx, db.GetAssetCurrentVersionParams{
		ID: assetID.String(), OwnerID: owner.String(),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("delete version: asset %s: %w", assetID, domain.ErrNotFound)
		}
		return false, fmt.Errorf("delete version: read current: %w", err)
	}
	if cur == versionID.String() {
		return false, nil // current version: refuse; caller maps to 409
	}
	if err := qtx.DeleteVersion(ctx, db.DeleteVersionParams{
		ID: versionID.String(), AssetID: assetID.String(), OwnerID: owner.String(),
	}); err != nil {
		return false, fmt.Errorf("delete version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit delete version: %w", err)
	}
	return true, nil
}

// createVersionTx inserts one version row on a transactional Queries handle.
func createVersionTx(ctx context.Context, q *db.Queries, v domain.AssetVersion) error {
	if err := q.CreateVersion(ctx, db.CreateVersionParams{
		ID:          v.ID.String(),
		AssetID:     v.AssetID.String(),
		OwnerID:     v.Owner.String(),
		VersionNo:   int64(v.VersionNo),
		Provider:    v.Provider,
		StoragePath: v.StoragePath,
		Hash:        v.Hash,
		Size:        v.Size,
		ThumbPath:   v.ThumbPath,
		Width:       int64(v.Width),
		Height:      int64(v.Height),
		Note:        v.Note,
		CreatedAt:   v.CreatedAt.Unix(),
	}); err != nil {
		return fmt.Errorf("create version %d: %w", v.VersionNo, err)
	}
	return nil
}

func rowToVersion(r db.AssetVersion) (domain.AssetVersion, error) {
	id, err := domain.NewVersionID(r.ID)
	if err != nil {
		return domain.AssetVersion{}, fmt.Errorf("row version id: %w", err)
	}
	assetID, err := domain.NewAssetID(r.AssetID)
	if err != nil {
		return domain.AssetVersion{}, fmt.Errorf("row version asset id: %w", err)
	}
	owner, err := domain.NewOwnerID(r.OwnerID)
	if err != nil {
		return domain.AssetVersion{}, fmt.Errorf("row version owner id: %w", err)
	}
	return domain.AssetVersion{
		ID:          id,
		AssetID:     assetID,
		Owner:       owner,
		VersionNo:   int(r.VersionNo),
		Provider:    r.Provider,
		StoragePath: r.StoragePath,
		Hash:        r.Hash,
		Size:        r.Size,
		ThumbPath:   r.ThumbPath,
		Width:       int(r.Width),
		Height:      int(r.Height),
		Note:        r.Note,
		CreatedAt:   time.Unix(r.CreatedAt, 0).UTC(),
	}, nil
}
