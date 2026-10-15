//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/service/agent/resultstore"

	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Runs the real SQLTool -> MySQL driver -> Redis -> exact read path. No row or
// column metadata is mocked; its random table and result keys are cleaned up.
func TestSQLExecuteColumnTypesMySQLRedisIntegration(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" || os.Getenv("REDIS_ADDR") == "" {
		t.Skip("MYSQL_DSN and REDIS_ADDR required")
	}
	config, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(config.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	raw, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	redisDB, err := strconv.Atoi(os.Getenv("REDIS_DB"))
	if os.Getenv("REDIS_DB") != "" {
		require.NoError(t, err)
	}
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR"), Password: os.Getenv("REDIS_PASSWORD"), DB: redisDB})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	store := resultstore.NewRedisStore(client)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx).Err())
	name := "result_types_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE TABLE `"+name+"` (id BIGINT PRIMARY KEY, code VARCHAR(20), amount DECIMAL(38,18), nullable_amount DECIMAL(38,18))").Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP TABLE IF EXISTS `"+name+"`").Error) })
	require.NoError(t, db.Exec("INSERT INTO `"+name+"` VALUES (?, ?, ?, NULL), (?, ?, ?, NULL)", int64(9007199254740993), "00123", "123.456789012345678901", int64(9007199254740994), "00007", "0.000000000000000009").Error)
	ctx = agentctx.WithSessionID(agentctx.WithTenantID(ctx, 1), name)
	owner := resultstore.OwnerKey(1, name)
	query := func(sql string) (*SQLExecuteResult, *resultstore.Result) {
		t.Helper()
		out, err := sqlExecute(ctx, &SQLExecuteRequest{SQL: sql}, db, nil, store, nil, nil)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, client.Del(context.Background(), "agent:resultstore:"+out.ResultID).Err()) })
		stored, err := store.GetExact(ctx, owner, out.ResultID)
		require.NoError(t, err)
		ordinary, err := store.Get(ctx, owner, out.ResultID)
		require.NoError(t, err)
		require.Equal(t, stored.ColumnTypes, ordinary.ColumnTypes)
		_, err = store.GetExact(ctx, resultstore.OwnerKey(2, name), out.ResultID)
		require.ErrorIs(t, err, resultstore.ErrUnauthorized)
		return out, stored
	}
	rows, stored := query("SELECT id, code, amount, nullable_amount FROM `" + name + "` ORDER BY id")
	require.Equal(t, "BIGINT", stored.ColumnTypes["id"])
	require.Equal(t, "VARCHAR", stored.ColumnTypes["code"])
	require.Equal(t, "DECIMAL", stored.ColumnTypes["amount"])
	require.Equal(t, json.Number("9007199254740993"), stored.Rows[0]["id"])
	require.Equal(t, "00123", stored.Rows[0]["code"])
	require.Equal(t, "123.456789012345678901", stored.Rows[0]["amount"])
	require.Nil(t, stored.Rows[0]["nullable_amount"])
	require.Equal(t, "string", rows.Dtypes["code"])
	_, aggregate := query("SELECT SUM(amount) AS revenue FROM `" + name + "`")
	require.Equal(t, "DECIMAL", aggregate.ColumnTypes["revenue"])
	require.Equal(t, "123.456789012345678910", aggregate.Rows[0]["revenue"])
}
