package store

import (
	"context"
	"database/sql"
	"fmt"
)

// migrateAssetColumns brings a pre-existing assets table up to the current
// column set before schema.sql is applied. CREATE TABLE IF NOT EXISTS never adds
// columns to an existing table, and schema.sql's indexes reference the newer
// columns, so a database created before a column existed must gain it via
// ALTER TABLE first. missing_at (issue #45) is added when the assets table
// exists but lacks the column; fresh databases (no assets table yet) and
// already-migrated ones are left untouched.
func migrateAssetColumns(ctx context.Context, sqldb *sql.DB) error {
	var cols int
	if err := sqldb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('assets')").Scan(&cols); err != nil {
		return fmt.Errorf("inspect assets columns: %w", err)
	}
	if cols == 0 {
		return nil // fresh database; schema.sql creates assets with all columns
	}
	var hasMissingAt int
	if err := sqldb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('assets') WHERE name = 'missing_at'").Scan(&hasMissingAt); err != nil {
		return fmt.Errorf("inspect missing_at column: %w", err)
	}
	if hasMissingAt > 0 {
		return nil // already migrated
	}
	if _, err := sqldb.ExecContext(ctx, "ALTER TABLE assets ADD COLUMN missing_at INTEGER"); err != nil {
		return fmt.Errorf("add missing_at column: %w", err)
	}
	return nil
}
