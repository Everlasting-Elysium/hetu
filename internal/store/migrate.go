package store

import (
	"context"
	"database/sql"
	"fmt"
)

// columnAdd is one idempotent "add this column if the table lacks it" step.
type columnAdd struct{ name, ddl string }

// addMissingColumns brings a pre-existing table up to the current column set
// before schema.sql is applied. CREATE TABLE IF NOT EXISTS never adds columns to
// an existing table, so a database created before a column existed must gain it
// via ALTER TABLE first. Each column is added only when the table exists but
// lacks it, so the migration is idempotent across restarts. A fresh database
// (the table not yet created) is left untouched — schema.sql creates it with the
// full column set. The table name is a package-internal literal (never user
// input), so it is safe to interpolate into the pragma query.
func addMissingColumns(ctx context.Context, sqldb *sql.DB, table string, cols []columnAdd) error {
	countAll := fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s')", table)
	var existing int
	if err := sqldb.QueryRowContext(ctx, countAll).Scan(&existing); err != nil {
		return fmt.Errorf("inspect %s columns: %w", table, err)
	}
	if existing == 0 {
		return nil // fresh database; schema.sql creates the table with all columns
	}
	countOne := fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?", table)
	for _, c := range cols {
		var has int
		if err := sqldb.QueryRowContext(ctx, countOne, c.name).Scan(&has); err != nil {
			return fmt.Errorf("inspect %s.%s column: %w", table, c.name, err)
		}
		if has > 0 {
			continue
		}
		if _, err := sqldb.ExecContext(ctx, c.ddl); err != nil {
			return fmt.Errorf("add %s.%s column: %w", table, c.name, err)
		}
	}
	return nil
}

// migrateAssetColumns adds columns introduced after the initial assets schema:
// missing_at (issue #45), current_version_id (issue #58), favorite and
// palette_manual (issue #62). Its indexes reference the newer columns, so this
// must run before schema.sql.
func migrateAssetColumns(ctx context.Context, sqldb *sql.DB) error {
	return addMissingColumns(ctx, sqldb, "assets", []columnAdd{
		{"missing_at", "ALTER TABLE assets ADD COLUMN missing_at INTEGER"},
		{"current_version_id", "ALTER TABLE assets ADD COLUMN current_version_id TEXT NOT NULL DEFAULT ''"},
		{"favorite", "ALTER TABLE assets ADD COLUMN favorite INTEGER NOT NULL DEFAULT 0"},
		{"palette_manual", "ALTER TABLE assets ADD COLUMN palette_manual INTEGER NOT NULL DEFAULT 0"},
	})
}

// migrateFolderColumns adds the cover/color columns introduced with folder
// covers (issue #62) so ListFoldersWithCover's f.cover/f.color references resolve
// on databases created before those columns existed.
func migrateFolderColumns(ctx context.Context, sqldb *sql.DB) error {
	return addMissingColumns(ctx, sqldb, "folders", []columnAdd{
		{"cover", "ALTER TABLE folders ADD COLUMN cover TEXT NOT NULL DEFAULT ''"},
		{"color", "ALTER TABLE folders ADD COLUMN color TEXT NOT NULL DEFAULT ''"},
	})
}
