// Package store persists hetu domain types. The v0 implementation is SQLite
// (pure-Go modernc driver, no CGO) behind the kernel.Store interface; a
// Postgres implementation can replace it without touching callers.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/kernel"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

//go:embed schema.sql
var schemaSQL string

// SQLite is the SQLite-backed kernel.Store.
type SQLite struct {
	sqldb *sql.DB
	q     *db.Queries
}

var _ kernel.Store = (*SQLite)(nil)

// Open opens (creating if needed) the database at path and applies the schema.
func Open(ctx context.Context, path string) (*SQLite, error) {
	sqldb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if _, err := sqldb.ExecContext(ctx, schemaSQL); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &SQLite{sqldb: sqldb, q: db.New(sqldb)}, nil
}

// Close closes the underlying database.
func (s *SQLite) Close() error {
	if err := s.sqldb.Close(); err != nil {
		return fmt.Errorf("close sqlite: %w", err)
	}
	return nil
}

// EnsureOwner inserts the owner row if it does not already exist.
func (s *SQLite) EnsureOwner(ctx context.Context, owner domain.OwnerID) error {
	if err := s.q.EnsureOwner(ctx, db.EnsureOwnerParams{
		ID:        owner.String(),
		CreatedAt: time.Now().Unix(),
	}); err != nil {
		return fmt.Errorf("ensure owner %s: %w", owner, err)
	}
	return nil
}

// UpsertAsset inserts or updates a, keyed by (owner, provider, storage_path).
func (s *SQLite) UpsertAsset(ctx context.Context, a domain.Asset) error {
	if err := s.q.UpsertAsset(ctx, db.UpsertAssetParams{
		ID:          a.ID.String(),
		OwnerID:     a.Owner.String(),
		Kind:        string(a.Kind),
		Provider:    a.Provider,
		StoragePath: a.StoragePath,
		Name:        a.Name,
		Ext:         a.Ext,
		Size:        a.Size,
		Hash:        a.Hash,
		ThumbPath:   a.ThumbPath,
		Width:       int64(a.Width),
		Height:      int64(a.Height),
		CreatedAt:   a.CreatedAt.Unix(),
		IndexedAt:   a.IndexedAt.Unix(),
	}); err != nil {
		return fmt.Errorf("upsert asset %s: %w", a.StoragePath, err)
	}
	return nil
}

// ListAssets returns the owner's assets, newest first.
func (s *SQLite) ListAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error) {
	rows, err := s.q.ListAssets(ctx, db.ListAssetsParams{
		OwnerID: owner.String(),
		Limit:   int64(limit),
		Offset:  int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	assets := make([]domain.Asset, 0, len(rows))
	for _, r := range rows {
		a, err := rowToAsset(r)
		if err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, nil
}

func rowToAsset(r db.Asset) (domain.Asset, error) {
	id, err := domain.NewAssetID(r.ID)
	if err != nil {
		return domain.Asset{}, fmt.Errorf("row asset id: %w", err)
	}
	owner, err := domain.NewOwnerID(r.OwnerID)
	if err != nil {
		return domain.Asset{}, fmt.Errorf("row owner id: %w", err)
	}
	return domain.Asset{
		ID:          id,
		Owner:       owner,
		Kind:        domain.AssetKind(r.Kind),
		Provider:    r.Provider,
		StoragePath: r.StoragePath,
		Name:        r.Name,
		Ext:         r.Ext,
		Size:        r.Size,
		Hash:        r.Hash,
		ThumbPath:   r.ThumbPath,
		Width:       int(r.Width),
		Height:      int(r.Height),
		CreatedAt:   time.Unix(r.CreatedAt, 0).UTC(),
		IndexedAt:   time.Unix(r.IndexedAt, 0).UTC(),
	}, nil
}
