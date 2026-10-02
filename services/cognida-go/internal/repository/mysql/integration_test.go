//go:build integration

package mysql

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"cognida/migrations"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// newIntegrationDB opens the explicitly configured MySQL instance shared by
// repository integration tests. It never supplies a fallback DSN: invoking
// these tests without MYSQL_DSN is a test-environment error, not an excuse to
// connect to an unintended database.
func newIntegrationDB(t testing.TB) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		host, port := os.Getenv("DB_HOST"), os.Getenv("DB_PORT")
		user, password, name := os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME")
		require.NotEmpty(t, host, "MYSQL_DSN or DB_HOST is required for MySQL integration tests")
		require.NotEmpty(t, port, "DB_PORT is required when MYSQL_DSN is unset")
		require.NotEmpty(t, user, "DB_USER is required when MYSQL_DSN is unset")
		require.NotEmpty(t, name, "DB_NAME is required when MYSQL_DSN is unset")
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, password, host, port, name)
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "connect MySQL")

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// Test the same schema shipped to production. AutoMigrate here would hide
	// missing migrations or silently change column types before assertions run.
	entries, err := migrations.FS.ReadDir(".")
	require.NoError(t, err)
	var latest uint64
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		version, err := strconv.ParseUint(strings.SplitN(entry.Name(), "_", 2)[0], 10, 64)
		require.NoError(t, err)
		if version > latest {
			latest = version
		}
	}
	var version uint64
	var dirty bool
	err = sqlDB.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	require.NoError(t, err, "run cmd/migrate-db up against the isolated integration database first")
	require.False(t, dirty, "integration database has a dirty migration")
	require.Equal(t, latest, version, "apply all versioned migrations before integration tests")
	return db
}
