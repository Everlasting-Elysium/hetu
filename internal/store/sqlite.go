// Package store persists hetu domain types. The v0 implementation is SQLite
// (pure-Go modernc driver, no CGO) behind the kernel.Store interface; a
// Postgres implementation can replace it without touching callers.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
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

// ftsSchemaVersion gates FTS migrations. Versions:
//   - 1: initial contentless FTS5 (name only)
//   - 2: non-contentless FTS5 with tags + description sync triggers
const ftsSchemaVersion = 2

// Open opens (creating if needed) the database at path and applies the schema.
func Open(ctx context.Context, path string) (*SQLite, error) {
	sqldb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if err := migrateFTS(ctx, sqldb); err != nil {
		_ = sqldb.Close()
		return nil, err
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

// migrateFTS handles schema transitions for the FTS index. When upgrading from
// v1 (contentless, name-only) to v2 (non-contentless, tags+description), it
// drops the old virtual table and triggers so schema.sql can recreate them with
// the new definition. Must run BEFORE schema.sql is applied because
// CREATE VIRTUAL TABLE IF NOT EXISTS would silently keep the old v1 table.
func migrateFTS(ctx context.Context, sqldb *sql.DB) error {
	var version int
	if err := sqldb.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version == 0 || version >= ftsSchemaVersion {
		return nil // fresh DB or already current
	}
	// Drop old triggers and FTS table so schema.sql recreates them.
	stmts := []string{
		"DROP TRIGGER IF EXISTS trg_assets_ai",
		"DROP TRIGGER IF EXISTS trg_assets_au",
		"DROP TRIGGER IF EXISTS trg_assets_ad",
		"DROP TABLE IF EXISTS assets_fts",
	}
	for _, s := range stmts {
		if _, err := sqldb.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("migrate fts drop: %w", err)
		}
	}
	return nil
}

// backfillFTS indexes all existing assets into assets_fts with their current
// tags and description. Runs once per schema version (gated by PRAGMA
// user_version). For a fresh database this is a harmless no-op.
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
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO assets_fts(rowid, name, tags, description)
		SELECT a.rowid, a.name,
			COALESCE((SELECT GROUP_CONCAT(t.name, ' ') FROM asset_tags at2 JOIN tags t ON t.id = at2.tag_id WHERE at2.asset_id = a.id), ''),
			COALESCE((SELECT ann.value FROM annotations ann WHERE ann.asset_id = a.id AND ann."key" = 'caption' ORDER BY CASE ann.layer WHEN 'manual' THEN 1 WHEN 'ai' THEN 2 ELSE 3 END LIMIT 1), '')
		FROM assets a`); err != nil {
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
// Re-indexing preserves user metadata (rating/color/display_name/folder_id and
// trash state): those columns are written on first insert and left untouched
// by the ON CONFLICT clause.
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
		DeletedAt:   timeToNullUnix(a.DeletedAt),
		Rating:      int64(a.Rating),
		Color:       a.Color,
		DisplayName: a.DisplayName,
		FolderID:    a.FolderID,
	}); err != nil {
		return fmt.Errorf("upsert asset %s: %w", a.StoragePath, err)
	}
	return nil
}

// GetAsset returns the owner's asset by id, or domain.ErrNotFound.
func (s *SQLite) GetAsset(ctx context.Context, owner domain.OwnerID, id domain.AssetID) (domain.Asset, error) {
	row, err := s.q.GetAsset(ctx, db.GetAssetParams{ID: id.String(), OwnerID: owner.String()})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Asset{}, fmt.Errorf("get asset %s: %w", id, domain.ErrNotFound)
		}
		return domain.Asset{}, fmt.Errorf("get asset %s: %w", id, err)
	}
	return rowToAsset(row)
}

// ListAssets returns the owner's live (non-trashed) assets, newest first.
func (s *SQLite) ListAssets(ctx context.Context, owner domain.OwnerID, limit, offset int) ([]domain.Asset, error) {
	rows, err := s.q.ListAssets(ctx, db.ListAssetsParams{
		OwnerID: owner.String(),
		Limit:   int64(limit),
		Offset:  int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	return rowsToAssets(rows)
}

func rowsToAssets(rows []db.Asset) ([]domain.Asset, error) {
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
// filters to the owner's live (non-trashed) assets, and orders by FTS5 bm25
// rank (ascending = best first). The column list mirrors db.Asset field order
// so the Scan below and rowToAsset stay aligned with sqlc's ListAssets.
const searchAssetsSQL = `
SELECT a.id, a.owner_id, a.kind, a.provider, a.storage_path, a.name, a.ext,
       a.size, a.hash, a.thumb_path, a.width, a.height, a.created_at, a.indexed_at,
       a.deleted_at, a.rating, a.color, a.display_name, a.folder_id
FROM assets_fts
JOIN assets a ON a.rowid = assets_fts.rowid
WHERE assets_fts MATCH ? AND a.owner_id = ? AND a.deleted_at IS NULL
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

	assets := []domain.Asset{}
	for rows.Next() {
		var r db.Asset
		if err := rows.Scan(
			&r.ID, &r.OwnerID, &r.Kind, &r.Provider, &r.StoragePath,
			&r.Name, &r.Ext, &r.Size, &r.Hash, &r.ThumbPath,
			&r.Width, &r.Height, &r.CreatedAt, &r.IndexedAt,
			&r.DeletedAt, &r.Rating, &r.Color, &r.DisplayName, &r.FolderID,
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
		DeletedAt:   nullUnixToTime(r.DeletedAt),
		Rating:      int(r.Rating),
		Color:       r.Color,
		DisplayName: r.DisplayName,
		FolderID:    r.FolderID,
	}, nil
}

// idStrings maps a slice of stringer IDs to their raw string values.
func idStrings[T interface{ String() string }](ids []T) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

func timeToNullUnix(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.Unix(), Valid: true}
}

func nullUnixToTime(n sql.NullInt64) *time.Time {
	if !n.Valid {
		return nil
	}
	t := time.Unix(n.Int64, 0).UTC()
	return &t
}
