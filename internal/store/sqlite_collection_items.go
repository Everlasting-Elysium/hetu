package store

import (
	"context"
	"fmt"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// AddCollectionItem appends assetID to the collection at the next ord (max+1, or
// 0 when empty). Re-adding an existing member updates its ord rather than
// erroring (the upsert keeps a member at most once). The MAX(ord) read and the
// insert share one transaction; SQLite serializes writers, so the next ord
// cannot be raced. Both the collection and the asset must belong to owner — the
// asset check exists so a crafted asset_id from another owner cannot be attached
// (and its kind/name/thumb then exposed via the enriched item list).
func (s *SQLite) AddCollectionItem(ctx context.Context, owner domain.OwnerID, collectionID domain.CollectionID, assetID domain.AssetID) error {
	if _, err := s.GetCollection(ctx, owner, collectionID); err != nil {
		return err
	}
	if _, err := s.GetAsset(ctx, owner, assetID); err != nil {
		return fmt.Errorf("add collection item: asset %s: %w", assetID, err)
	}
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin add collection item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)
	next, err := q.NextCollectionItemOrd(ctx, collectionID.String())
	if err != nil {
		return fmt.Errorf("next collection item ord: %w", err)
	}
	if err := q.AddCollectionItem(ctx, db.AddCollectionItemParams{
		CollectionID: collectionID.String(),
		AssetID:      assetID.String(),
		Ord:          next,
	}); err != nil {
		return fmt.Errorf("add collection item %s: %w", assetID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add collection item: %w", err)
	}
	return nil
}

// RemoveCollectionItem detaches assetID from the collection and, in the same
// transaction, clears an explicit cover override that pointed at it — a removed
// member must never linger as the collection's cover. The collection must
// belong to owner.
func (s *SQLite) RemoveCollectionItem(ctx context.Context, owner domain.OwnerID, collectionID domain.CollectionID, assetID domain.AssetID) error {
	if _, err := s.GetCollection(ctx, owner, collectionID); err != nil {
		return err
	}
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remove collection item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)
	if err := q.RemoveCollectionItem(ctx, db.RemoveCollectionItemParams{
		CollectionID: collectionID.String(),
		AssetID:      assetID.String(),
	}); err != nil {
		return fmt.Errorf("remove collection item %s: %w", assetID, err)
	}
	if err := q.ClearCollectionCoverIfMatches(ctx, db.ClearCollectionCoverIfMatchesParams{
		ID:    collectionID.String(),
		Cover: assetID.String(),
	}); err != nil {
		return fmt.Errorf("clear stale cover for %s: %w", assetID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove collection item: %w", err)
	}
	return nil
}

// ListCollectionItems returns the collection's members ordered by ord, each
// enriched with its asset's kind/name/thumb via a single JOIN (asset display
// name overrides the file name when set). The collection must belong to owner.
func (s *SQLite) ListCollectionItems(ctx context.Context, owner domain.OwnerID, collectionID domain.CollectionID) ([]domain.CollectionItem, error) {
	if _, err := s.GetCollection(ctx, owner, collectionID); err != nil {
		return nil, err
	}
	rows, err := s.q.ListCollectionItemsEnriched(ctx, collectionID.String())
	if err != nil {
		return nil, fmt.Errorf("list collection items: %w", err)
	}
	items := make([]domain.CollectionItem, 0, len(rows))
	for _, r := range rows {
		aid, err := domain.NewAssetID(r.AssetID)
		if err != nil {
			return nil, fmt.Errorf("row collection item asset id: %w", err)
		}
		name := r.Name
		if r.DisplayName != "" {
			name = r.DisplayName
		}
		items = append(items, domain.CollectionItem{
			AssetID:    aid,
			Ord:        int(r.Ord),
			AssetKind:  r.Kind,
			AssetName:  name,
			AssetThumb: r.ThumbPath,
		})
	}
	return items, nil
}

// ReorderCollectionItems rewrites every member's ord to its index in
// assetIDsInOrder (0..n-1) in one transaction, after verifying the given set
// matches the collection's current members exactly. The collection must belong
// to owner.
func (s *SQLite) ReorderCollectionItems(ctx context.Context, owner domain.OwnerID, collectionID domain.CollectionID, assetIDsInOrder []domain.AssetID) error {
	if _, err := s.GetCollection(ctx, owner, collectionID); err != nil {
		return err
	}
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reorder collection items: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)
	current, err := q.ListCollectionItemAssetIDs(ctx, collectionID.String())
	if err != nil {
		return fmt.Errorf("list collection item ids: %w", err)
	}
	if err := verifySameMembers(current, assetIDsInOrder); err != nil {
		return err
	}
	for i, aid := range assetIDsInOrder {
		if err := q.SetCollectionItemOrd(ctx, db.SetCollectionItemOrdParams{
			Ord:          int64(i),
			CollectionID: collectionID.String(),
			AssetID:      aid.String(),
		}); err != nil {
			return fmt.Errorf("set collection item ord %s: %w", aid, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reorder collection items: %w", err)
	}
	return nil
}

// verifySameMembers checks that want lists exactly the assets in current — same
// count, same membership, no duplicates — returning ErrCollectionItemsMismatch
// otherwise, so a reorder can never drop, add, or duplicate a member.
func verifySameMembers(current []string, want []domain.AssetID) error {
	if len(current) != len(want) {
		return fmt.Errorf("reorder set has %d items, collection has %d: %w", len(want), len(current), domain.ErrCollectionItemsMismatch)
	}
	members := make(map[string]bool, len(current))
	for _, id := range current {
		members[id] = true
	}
	seen := make(map[string]bool, len(want))
	for _, aid := range want {
		id := aid.String()
		if !members[id] {
			return fmt.Errorf("reorder asset %s is not a collection member: %w", id, domain.ErrCollectionItemsMismatch)
		}
		if seen[id] {
			return fmt.Errorf("reorder asset %s appears twice: %w", id, domain.ErrCollectionItemsMismatch)
		}
		seen[id] = true
	}
	return nil
}
