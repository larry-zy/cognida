package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"
)

// WithDocumentLock holds a MySQL advisory lock on one dedicated connection.
// Connection loss/process death releases it; it cannot expire under live work.
func (r *knowledgeRepository) WithDocumentLock(ctx context.Context, key string, fn func() error) error {
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 10)", key).Scan(&acquired); err != nil {
		return err
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("document ingestion is already running")
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanupCtx, "DO RELEASE_LOCK(?)", key); err != nil {
			// Do not return a connection holding a session lock to the pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	return fn()
}
