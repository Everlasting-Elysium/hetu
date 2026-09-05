// Package store persists hetu domain types. The v0 implementation is SQLite
// (pure-Go modernc driver, no CGO) behind the kernel.Store interface; a
// Postgres implementation can replace it without touching callers.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
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

// ftsSchemaVersion gates the one-time backfill of assets_fts. schema.sql is
// re-applied on every Open (IF NOT EXISTS), so the backfill must run exactly
// once — otherwise it would double-index rows already in the FTS table.
const ftsSchemaVersion = 1

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
	if err := backfillFTS(ctx, sqldb); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	return &SQLite{sqldb: sqldb, q: db.New(sqldb)}, nil
}

// backfillFTS indexes assets rows that predate the assets_fts table. A database
// created before FTS existed has the table+triggers added by schema.sql but no
// indexed rows; the first upsert would then fire the UPDATE trigger's
// contentless 'delete' against an unindexed row and corrupt the database. This
// runs once (gated by PRAGMA user_version) to seed the index; for a fresh DB it
// is a harmless no-op. content='' forfeits the FTS5 'rebuild' command, so the
// backfill is a manual INSERT..SELECT.
func backfillFTS(ctx context.Context, sqldb *sql.DB) error {
	var version int
	if err := sqldb.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version >= ftsSchemaVersion {
		return nil
	}
	tx, err := sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fts backfill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO assets_fts(rowid, name, tags, description)
		 SELECT rowid, name, '', '' FROM assets`); err != nil {
		return fmt.Errorf("backfill assets_fts: %w", err)
	}
	// PRAGMA does not accept bound parameters; the value is a trusted constant.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", ftsSchemaVersion)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fts backfill: %w", err)
	}
	return nil
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

// searchAssetsSQL is hand-written because sqlc does not support FTS5 virtual
// tables. The query joins assets_fts (matched by MATCH) with assets via rowid,
// filters by owner_id, and orders by FTS5 bm25 rank (ascending = best first).
const searchAssetsSQL = `
SELECT a.id, a.owner_id, a.kind, a.provider, a.storage_path, a.name, a.ext,
       a.size, a.hash, a.thumb_path, a.width, a.height, a.created_at, a.indexed_at
FROM assets_fts
JOIN assets a ON a.rowid = assets_fts.rowid
WHERE assets_fts MATCH ? AND a.owner_id = ?
ORDER BY assets_fts.rank
LIMIT ? OFFSET ?`

// SearchAssets performs FTS5 full-text search. ftsQuery must be a valid FTS5
// MATCH expression (produced by the search package parser).
func (s *SQLite) SearchAssets(ctx context.Context, owner domain.OwnerID, ftsQuery string, limit, offset int) ([]domain.Asset, error) {
	rows, err := s.sqldb.QueryContext(ctx, searchAssetsSQL, ftsQuery, owner.String(), limit, offset)
	if err != nil {
		if isFTSQueryError(err) {
			return nil, fmt.Errorf("%w: %s", domain.ErrInvalidQuery, err.Error())
		}
		return nil, fmt.Errorf("search assets: %w", err)
	}
	defer rows.Close()

	var assets []domain.Asset
	for rows.Next() {
		var r db.Asset
		if err := rows.Scan(
			&r.ID, &r.OwnerID, &r.Kind, &r.Provider, &r.StoragePath,
			&r.Name, &r.Ext, &r.Size, &r.Hash, &r.ThumbPath,
			&r.Width, &r.Height, &r.CreatedAt, &r.IndexedAt,
		); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		a, err := rowToAsset(r)
		if err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search rows iteration: %w", err)
	}
	if assets == nil {
		assets = []domain.Asset{}
	}
	return assets, nil
}

// isFTSQueryError reports whether err is an FTS5 MATCH query error (malformed
// user query) rather than a server-side fault. modernc surfaces these as a
// generic SQLITE_ERROR, so detection is by message. The search package parser
// prevents these, so this is defense-in-depth against parser regressions.
func isFTSQueryError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "fts5: syntax error") ||
		strings.Contains(msg, "unknown special query")
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
