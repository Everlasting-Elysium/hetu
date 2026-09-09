package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// maxCollectionDepth bounds the ancestor-chain walk in validateParent, guarding
// against an unbounded loop if pre-existing data were ever corrupt (a parent_id
// chain that never reaches root). Collection nesting in practice is a handful
// of levels, so this is far above any legitimate depth.
const maxCollectionDepth = 100

// validateParent rejects a parentID that would make id its own parent or an
// ancestor of itself (a cycle), and confirms a non-empty parentID names an
// existing, owner-scoped collection (GetCollection already 404s otherwise). A
// fresh id (create path, not yet persisted) can never appear in the walk, so
// this is a no-op cycle-wise for creation and only bites on reparenting via
// UpdateCollection.
func (s *SQLite) validateParent(ctx context.Context, owner domain.OwnerID, id domain.CollectionID, parentID string) error {
	if parentID == "" {
		return nil
	}
	if parentID == id.String() {
		return fmt.Errorf("collection %s cannot be its own parent: %w", id, domain.ErrCollectionCycle)
	}
	pid, err := domain.NewCollectionID(parentID)
	if err != nil {
		return fmt.Errorf("parent id: %w", err)
	}
	cur, err := s.GetCollection(ctx, owner, pid)
	if err != nil {
		return fmt.Errorf("parent collection %s: %w", parentID, err)
	}
	for range maxCollectionDepth {
		if cur.ID == id {
			return fmt.Errorf("collection %s: parent %s is its own descendant: %w", id, parentID, domain.ErrCollectionCycle)
		}
		if cur.ParentID == "" {
			return nil
		}
		nextID, err := domain.NewCollectionID(cur.ParentID)
		if err != nil {
			return nil // dangling ancestor ref is a separate row's problem, not this call's
		}
		next, err := s.GetCollection(ctx, owner, nextID)
		if err != nil {
			return nil // ancestor chain broken elsewhere — nothing to cycle back to id
		}
		cur = next
	}
	return fmt.Errorf("collection %s: parent chain exceeds depth %d: %w", id, maxCollectionDepth, domain.ErrCollectionCycle)
}

// CreateCollection inserts a new collection, rejecting a non-empty parent_id
// that does not name an existing, owner-scoped collection.
func (s *SQLite) CreateCollection(ctx context.Context, c domain.Collection) error {
	if err := s.validateParent(ctx, c.Owner, c.ID, c.ParentID); err != nil {
		return err
	}
	if err := s.q.CreateCollection(ctx, db.CreateCollectionParams{
		ID:       c.ID.String(),
		OwnerID:  c.Owner.String(),
		ParentID: c.ParentID,
		Name:     c.Name,
		Cover:    c.Cover,
	}); err != nil {
		return fmt.Errorf("create collection %s: %w", c.Name, err)
	}
	return nil
}

// ListCollections returns the owner's collections ordered by name, each carrying
// its resolved effective cover: the explicit override, else the lowest-ord
// member's asset id, else empty (see queries/collection.sql).
func (s *SQLite) ListCollections(ctx context.Context, owner domain.OwnerID) ([]domain.Collection, error) {
	rows, err := s.q.ListCollectionsWithCover(ctx, owner.String())
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	cols := make([]domain.Collection, 0, len(rows))
	for _, r := range rows {
		id, err := domain.NewCollectionID(r.ID)
		if err != nil {
			return nil, fmt.Errorf("row collection id: %w", err)
		}
		own, err := domain.NewOwnerID(r.OwnerID)
		if err != nil {
			return nil, fmt.Errorf("row collection owner: %w", err)
		}
		cols = append(cols, domain.Collection{
			ID:       id,
			Owner:    own,
			ParentID: r.ParentID,
			Name:     r.Name,
			Cover:    r.EffectiveCover,
		})
	}
	return cols, nil
}

// GetCollection returns a collection by id with its RAW stored cover (not the
// resolved effective cover), or domain.ErrNotFound.
func (s *SQLite) GetCollection(ctx context.Context, owner domain.OwnerID, id domain.CollectionID) (domain.Collection, error) {
	row, err := s.q.GetCollection(ctx, db.GetCollectionParams{ID: id.String(), OwnerID: owner.String()})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Collection{}, fmt.Errorf("get collection %s: %w", id, domain.ErrNotFound)
		}
		return domain.Collection{}, fmt.Errorf("get collection %s: %w", id, err)
	}
	return rowToCollection(row)
}

// UpdateCollection sets a collection's name, parent_id, and cover. A non-empty
// parentID must name an existing, owner-scoped collection and must not create a
// cycle (id becoming its own ancestor), or domain.ErrCollectionCycle/ErrNotFound
// is returned. A non-empty cover must be a current member (verified against
// collection_items) or a wrapped domain.ErrNotFound is returned; an empty cover
// clears the override so the effective cover falls back to the lowest-ord member.
func (s *SQLite) UpdateCollection(ctx context.Context, owner domain.OwnerID, id domain.CollectionID, name, parentID, cover string) error {
	if _, err := s.GetCollection(ctx, owner, id); err != nil {
		return err
	}
	if err := s.validateParent(ctx, owner, id, parentID); err != nil {
		return err
	}
	if cover != "" {
		n, err := s.q.IsCollectionMember(ctx, db.IsCollectionMemberParams{
			CollectionID: id.String(),
			AssetID:      cover,
		})
		if err != nil {
			return fmt.Errorf("check cover member %s: %w", cover, err)
		}
		if n == 0 {
			return fmt.Errorf("cover asset %s not a member of collection %s: %w", cover, id, domain.ErrNotFound)
		}
	}
	if err := s.q.UpdateCollection(ctx, db.UpdateCollectionParams{
		Name:     name,
		ParentID: parentID,
		Cover:    cover,
		ID:       id.String(),
		OwnerID:  owner.String(),
	}); err != nil {
		return fmt.Errorf("update collection %s: %w", id, err)
	}
	return nil
}

// DeleteCollection removes a collection and its membership rows in one
// transaction so no orphaned collection_items remain (unlike folders/tags, whose
// association tables are left to independent cleanup).
func (s *SQLite) DeleteCollection(ctx context.Context, owner domain.OwnerID, id domain.CollectionID) error {
	tx, err := s.sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete collection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)
	if err := q.DeleteCollectionItemsByCollection(ctx, id.String()); err != nil {
		return fmt.Errorf("delete collection items for %s: %w", id, err)
	}
	if err := q.DeleteCollection(ctx, db.DeleteCollectionParams{ID: id.String(), OwnerID: owner.String()}); err != nil {
		return fmt.Errorf("delete collection %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete collection: %w", err)
	}
	return nil
}

func rowToCollection(r db.Collection) (domain.Collection, error) {
	id, err := domain.NewCollectionID(r.ID)
	if err != nil {
		return domain.Collection{}, fmt.Errorf("row collection id: %w", err)
	}
	owner, err := domain.NewOwnerID(r.OwnerID)
	if err != nil {
		return domain.Collection{}, fmt.Errorf("row collection owner: %w", err)
	}
	return domain.Collection{
		ID:       id,
		Owner:    owner,
		ParentID: r.ParentID,
		Name:     r.Name,
		Cover:    r.Cover,
	}, nil
}
