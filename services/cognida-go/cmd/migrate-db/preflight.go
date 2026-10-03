package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Old development builds used AutoMigrate for Collection without recording a
// migration version. Detect that state before Up changes anything: migration 5
// cannot safely adopt those nullable, unindexed tables with CREATE IF NOT EXISTS.
// In particular, force 5 would silently skip both schema repair and migration 4.
func preflightCollection(ctx context.Context, db *sql.DB, version uint) error {
	if version >= 5 {
		return nil
	}
	rows, err := db.QueryContext(ctx, `SELECT table_name FROM information_schema.tables
WHERE table_schema=DATABASE() AND table_name IN ('collection_tasks','collection_sources','collection_runs','collection_items')
ORDER BY table_name`)
	if err != nil {
		return fmt.Errorf("inspect legacy collection schema: %w", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(tables) > 0 {
		return fmt.Errorf("version %d predates Collection migration 5 but tables already exist: %s; back up and reconcile these legacy tables using migrations/README.md before retrying; no migration has been applied", version, strings.Join(tables, ", "))
	}
	return nil
}
