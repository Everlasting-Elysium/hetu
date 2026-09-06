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
// ALTER TABLE first. Each column is added only when the assets table exists but
// lacks it: missing_at (issue #45) and current_version_id (issue #58). Fresh
// databases (no assets table yet) and already-migrated ones are left untouched.
func migrateAssetColumns(ctx context.Context, sqldb *sql.DB) error {
	var cols int
	if err := sqldb.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('assets')").Scan(&cols); err != nil {
		return fmt.Errorf("inspect assets columns: %w", err)
	}
	if cols == 0 {
		return nil // fresh database; schema.sql creates assets with all columns
	}
	// addColumns is applied in order; each entry is skipped when the column is
	// already present, so the migration is idempotent across restarts.
	addColumns := []struct{ name, ddl string }{
		{"missing_at", "ALTER TABLE assets ADD COLUMN missing_at INTEGER"},
		{"current_version_id", "ALTER TABLE assets ADD COLUMN current_version_id TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range addColumns {
		var has int
		if err := sqldb.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM pragma_table_info('assets') WHERE name = ?", c.name).Scan(&has); err != nil {
			return fmt.Errorf("inspect %s column: %w", c.name, err)
		}
		if has > 0 {
			continue
		}
		if _, err := sqldb.ExecContext(ctx, c.ddl); err != nil {
			return fmt.Errorf("add %s column: %w", c.name, err)
		}
	}
	return nil
}
