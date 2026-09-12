package store

import (
	"context"
	"database/sql"
	"errors"
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

// getTag loads a single tag scoped to owner, mapping a missing row to
// domain.ErrNotFound so callers can 404 an unknown or foreign tag.
func (s *SQLite) getTag(ctx context.Context, owner domain.OwnerID, id domain.TagID) (db.Tag, error) {
	row, err := s.q.GetTag(ctx, db.GetTagParams{ID: id.String(), OwnerID: owner.String()})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.Tag{}, fmt.Errorf("get tag %s: %w", id, domain.ErrNotFound)
		}
		return db.Tag{}, fmt.Errorf("get tag %s: %w", id, err)
	}
	return row, nil
}

// MergeTags folds fromTagID into intoTagID for the whole owner: it re-hangs
// every asset carrying fromTagID onto intoTagID (INSERT OR IGNORE dedups against
// the asset_tags (asset_id, tag_id) primary key so an asset that already has
// both keeps a single intoTagID row), promotes fromTagID's direct children up to
// fromTagID's own parent (so no child is left pointing at the deleted tag), then
// removes the leftover fromTagID asset_tags rows and the tag itself — all in one
// transaction, so a partial merge can never be observed. This is a global,
// destructive operation: it affects every asset, not a selection.
func (s *SQLite) MergeTags(ctx context.Context, owner domain.OwnerID, fromTagID, intoTagID domain.TagID) error {
	if fromTagID == intoTagID {
		return fmt.Errorf("merge tag %s into itself: %w", fromTagID, domain.ErrSameTag)
	}
	from, err := s.getTag(ctx, owner, fromTagID)
	if err != nil {
		return err
	}
	if _, err := s.getTag(ctx, owner, intoTagID); err != nil {
		return err
	}
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin merge tags tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)
	// Promote fromTagID's children to its parent before deleting it. from.ParentID
	// is empty for a top-level tag, which promotes the children to top level.
	if err := qtx.ReparentTagChildren(ctx, db.ReparentTagChildrenParams{
		NewParentID: from.ParentID,
		OldParentID: fromTagID.String(),
		OwnerID:     owner.String(),
	}); err != nil {
		return fmt.Errorf("reparent children of %s: %w", fromTagID, err)
	}
	if err := qtx.ReattachAssetTags(ctx, db.ReattachAssetTagsParams{
		IntoTagID: intoTagID.String(),
		FromTagID: fromTagID.String(),
	}); err != nil {
		return fmt.Errorf("reattach asset tags %s -> %s: %w", fromTagID, intoTagID, err)
	}
	if err := qtx.DeleteAssetTagsByTag(ctx, fromTagID.String()); err != nil {
		return fmt.Errorf("delete leftover asset tags %s: %w", fromTagID, err)
	}
	if err := qtx.DeleteTag(ctx, db.DeleteTagParams{ID: fromTagID.String(), OwnerID: owner.String()}); err != nil {
		return fmt.Errorf("delete merged tag %s: %w", fromTagID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit merge tags tx: %w", err)
	}
	return nil
}

// BatchReplaceTag swaps fromTagID for toTagID on the given assets only: it
// re-hangs those assets' fromTagID rows onto toTagID (INSERT OR IGNORE dedups
// via the asset_tags primary key) and then deletes the fromTagID rows within the
// same subset — in one transaction. fromTagID is NOT deleted from the tags table
// (assets outside the selection may still carry it), which is the deliberate
// difference from MergeTags. owner is reserved for multi-user scoping.
func (s *SQLite) BatchReplaceTag(ctx context.Context, _ domain.OwnerID, assetIDs []domain.AssetID, fromTagID, toTagID domain.TagID) error {
	if fromTagID == toTagID {
		return fmt.Errorf("replace tag %s with itself: %w", fromTagID, domain.ErrSameTag)
	}
	ids := idStrings(assetIDs)
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace tag tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)
	if err := qtx.ReattachAssetTagsForAssets(ctx, db.ReattachAssetTagsForAssetsParams{
		IntoTagID: toTagID.String(),
		FromTagID: fromTagID.String(),
		AssetIds:  ids,
	}); err != nil {
		return fmt.Errorf("reattach asset tags %s -> %s: %w", fromTagID, toTagID, err)
	}
	if err := qtx.BatchRemoveTags(ctx, db.BatchRemoveTagsParams{
		AssetIds: ids,
		TagID:    fromTagID.String(),
	}); err != nil {
		return fmt.Errorf("remove replaced tag %s: %w", fromTagID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace tag tx: %w", err)
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
