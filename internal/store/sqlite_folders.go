package store

import (
	"context"
	"fmt"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// CreateFolder inserts a new folder.
func (s *SQLite) CreateFolder(ctx context.Context, f domain.Folder) error {
	if err := s.q.CreateFolder(ctx, db.CreateFolderParams{
		ID:       f.ID.String(),
		OwnerID:  f.Owner.String(),
		ParentID: f.ParentID,
		Name:     f.Name,
		Path:     f.Path,
	}); err != nil {
		return fmt.Errorf("create folder %s: %w", f.Name, err)
	}
	return nil
}

// ListFolders returns the owner's folders ordered by path.
func (s *SQLite) ListFolders(ctx context.Context, owner domain.OwnerID) ([]domain.Folder, error) {
	rows, err := s.q.ListFolders(ctx, owner.String())
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
		})
	}
	return folders, nil
}

// DeleteFolder removes a folder owned by owner.
func (s *SQLite) DeleteFolder(ctx context.Context, owner domain.OwnerID, id domain.FolderID) error {
	if err := s.q.DeleteFolder(ctx, db.DeleteFolderParams{ID: id.String(), OwnerID: owner.String()}); err != nil {
		return fmt.Errorf("delete folder %s: %w", id, err)
	}
	return nil
}
