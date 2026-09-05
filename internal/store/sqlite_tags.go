package store

import (
	"context"
	"fmt"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// tagSourceManual marks a tag applied by a user (vs. an AI tagging pipeline).
const tagSourceManual = "manual"

// CreateTag inserts a new tag.
func (s *SQLite) CreateTag(ctx context.Context, t domain.Tag) error {
	if err := s.q.CreateTag(ctx, db.CreateTagParams{
		ID:       t.ID.String(),
		OwnerID:  t.Owner.String(),
		ParentID: t.ParentID,
		Name:     t.Name,
		Color:    t.Color,
	}); err != nil {
		return fmt.Errorf("create tag %s: %w", t.Name, err)
	}
	return nil
}

// ListTags returns the owner's tags ordered by name.
func (s *SQLite) ListTags(ctx context.Context, owner domain.OwnerID) ([]domain.Tag, error) {
	rows, err := s.q.ListTags(ctx, owner.String())
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	return rowsToTags(rows)
}

// DeleteTag removes a tag owned by owner.
func (s *SQLite) DeleteTag(ctx context.Context, owner domain.OwnerID, id domain.TagID) error {
	if err := s.q.DeleteTag(ctx, db.DeleteTagParams{ID: id.String(), OwnerID: owner.String()}); err != nil {
		return fmt.Errorf("delete tag %s: %w", id, err)
	}
	return nil
}

// BatchAddTags attaches every tag in tagIDs to every asset in assetIDs
// (idempotent) within one transaction. owner is reserved for multi-user scoping.
func (s *SQLite) BatchAddTags(ctx context.Context, _ domain.OwnerID, assetIDs []domain.AssetID, tagIDs []domain.TagID) error {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin add tags tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)
	for _, aid := range assetIDs {
		for _, tid := range tagIDs {
			if err := qtx.AddAssetTag(ctx, db.AddAssetTagParams{
				AssetID: aid.String(),
				TagID:   tid.String(),
				Source:  tagSourceManual,
			}); err != nil {
				return fmt.Errorf("add asset tag: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add tags tx: %w", err)
	}
	return nil
}

// BatchRemoveTags detaches tagID from every asset in assetIDs. owner is reserved
// for multi-user scoping.
func (s *SQLite) BatchRemoveTags(ctx context.Context, _ domain.OwnerID, assetIDs []domain.AssetID, tagID domain.TagID) error {
	if err := s.q.BatchRemoveTags(ctx, db.BatchRemoveTagsParams{
		AssetIds: idStrings(assetIDs),
		TagID:    tagID.String(),
	}); err != nil {
		return fmt.Errorf("batch remove tags: %w", err)
	}
	return nil
}

// ListAssetTags returns the tags attached to a single asset, ordered by name.
func (s *SQLite) ListAssetTags(ctx context.Context, assetID domain.AssetID) ([]domain.Tag, error) {
	rows, err := s.q.ListAssetTags(ctx, assetID.String())
	if err != nil {
		return nil, fmt.Errorf("list asset tags: %w", err)
	}
	return rowsToTags(rows)
}

func rowsToTags(rows []db.Tag) ([]domain.Tag, error) {
	tags := make([]domain.Tag, 0, len(rows))
	for _, r := range rows {
		id, err := domain.NewTagID(r.ID)
		if err != nil {
			return nil, fmt.Errorf("row tag id: %w", err)
		}
		owner, err := domain.NewOwnerID(r.OwnerID)
		if err != nil {
			return nil, fmt.Errorf("row tag owner: %w", err)
		}
		tags = append(tags, domain.Tag{
			ID:       id,
			Owner:    owner,
			ParentID: r.ParentID,
			Name:     r.Name,
			Color:    r.Color,
		})
	}
	return tags, nil
}
