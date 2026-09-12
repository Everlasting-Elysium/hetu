package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// validateFolderCover rejects a non-empty cover that does not name a live
// (non-trashed) asset owned by owner, guarding folder cover writes against
// dangling references and cross-owner (IDOR) references. An empty cover clears
// the override so the effective cover falls back to the earliest-indexed asset.
func (s *SQLite) validateFolderCover(ctx context.Context, owner domain.OwnerID, cover string) error {
	if cover == "" {
		return nil
	}
	n, err := s.q.CountOwnedLiveAsset(ctx, db.CountOwnedLiveAssetParams{
		ID:      cover,
		OwnerID: owner.String(),
	})
	if err != nil {
		return fmt.Errorf("check folder cover asset %s: %w", cover, err)
	}
	if n == 0 {
		return fmt.Errorf("cover asset %s not a live asset of owner: %w", cover, domain.ErrNotFound)
	}
	return nil
}

// CreateFolder inserts a new folder, rejecting a non-empty cover that is not a
// live, owner-scoped asset.
func (s *SQLite) CreateFolder(ctx context.Context, f domain.Folder) error {
	if err := s.validateFolderCover(ctx, f.Owner, f.Cover); err != nil {
		return err
	}
	if err := s.q.CreateFolder(ctx, db.CreateFolderParams{
		ID:       f.ID.String(),
		OwnerID:  f.Owner.String(),
		ParentID: f.ParentID,
		Name:     f.Name,
		Path:     f.Path,
		Cover:    f.Cover,
		Color:    f.Color,
	}); err != nil {
		return fmt.Errorf("create folder %s: %w", f.Name, err)
	}
	return nil
}

// ListFolders returns the owner's folders ordered by path, each carrying its
// resolved effective cover: the explicit override, else the earliest-indexed
// live asset in the folder, else empty (see queries/folder.sql).
func (s *SQLite) ListFolders(ctx context.Context, owner domain.OwnerID) ([]domain.Folder, error) {
	rows, err := s.q.ListFoldersWithCover(ctx, owner.String())
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	folders := make([]domain.Folder, 0, len(rows))
	for _, r := range rows {
		id, err := domain.NewFolderID(r.ID)
		if err != nil {
			return nil, fmt.Errorf("row folder id: %w", err)
		}
		own, err := domain.NewOwnerID(r.OwnerID)
		if err != nil {
			return nil, fmt.Errorf("row folder owner: %w", err)
		}
		folders = append(folders, domain.Folder{
			ID:       id,
			Owner:    own,
			ParentID: r.ParentID,
			Name:     r.Name,
			Path:     r.Path,
			Cover:    r.EffectiveCover,
			Color:    r.Color,
		})
	}
	return folders, nil
}

// GetFolder returns a folder by id with its RAW stored cover (not the resolved
// effective cover), or domain.ErrNotFound.
func (s *SQLite) GetFolder(ctx context.Context, owner domain.OwnerID, id domain.FolderID) (domain.Folder, error) {
	row, err := s.q.GetFolder(ctx, db.GetFolderParams{ID: id.String(), OwnerID: owner.String()})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Folder{}, fmt.Errorf("get folder %s: %w", id, domain.ErrNotFound)
		}
		return domain.Folder{}, fmt.Errorf("get folder %s: %w", id, err)
	}
	own, err := domain.NewOwnerID(row.OwnerID)
	if err != nil {
		return domain.Folder{}, fmt.Errorf("row folder owner: %w", err)
	}
	return domain.Folder{
		ID:       id,
		Owner:    own,
		ParentID: row.ParentID,
		Name:     row.Name,
		Path:     row.Path,
		Cover:    row.Cover,
		Color:    row.Color,
	}, nil
}

// UpdateFolderCover sets a folder's cover override and color. A non-empty cover
// must be a live, owner-scoped asset (or a wrapped domain.ErrNotFound is
// returned); an empty cover clears the override so the effective cover falls back
// to the earliest-indexed asset. The folder must exist or domain.ErrNotFound is
// returned.
func (s *SQLite) UpdateFolderCover(ctx context.Context, owner domain.OwnerID, id domain.FolderID, cover, color string) error {
	if _, err := s.GetFolder(ctx, owner, id); err != nil {
		return err
	}
	if err := s.validateFolderCover(ctx, owner, cover); err != nil {
		return err
	}
	if err := s.q.UpdateFolderCover(ctx, db.UpdateFolderCoverParams{
		Cover:   cover,
		Color:   color,
		ID:      id.String(),
		OwnerID: owner.String(),
	}); err != nil {
		return fmt.Errorf("update folder %s: %w", id, err)
	}
	return nil
}

// DeleteFolder removes a folder owned by owner.
func (s *SQLite) DeleteFolder(ctx context.Context, owner domain.OwnerID, id domain.FolderID) error {
	if err := s.q.DeleteFolder(ctx, db.DeleteFolderParams{ID: id.String(), OwnerID: owner.String()}); err != nil {
		return fmt.Errorf("delete folder %s: %w", id, err)
	}
	return nil
}
