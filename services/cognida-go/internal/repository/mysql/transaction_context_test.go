// Package mysql: 事务上下文传播的单元测试。
// 用 go-sqlmock 构造两个独立的 *gorm.DB，验证 DBFromContext 在有/无事务、
// 以及遭遇遗留字符串 key 时的路由行为——这是"事务真正落地"的核心保证。
package mysql

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// newMockGormDB 基于 sqlmock 构造一个不连真库的 *gorm.DB。
func newMockGormDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	return gdb, mock
}

// TestDBFromContext_UsesTxWhenPresent 验证：ctx 携带事务句柄时，DBFromContext
// 把操作路由到该事务连接，而不是 fallback 连接。这正是旧实现失效的地方。
func TestDBFromContext_UsesTxWhenPresent(t *testing.T) {
	fallback, fallbackMock := newMockGormDB(t)
	txDB, txMock := newMockGormDB(t)

	// 期望：写操作只落在 tx 连接上；fallback 连接不应被触碰。
	txMock.ExpectExec("UPDATE demo").WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := contextWithTx(context.Background(), txDB)
	err := DBFromContext(ctx, fallback).Exec("UPDATE demo SET x = 1").Error

	require.NoError(t, err)
	assert.NoError(t, txMock.ExpectationsWereMet(), "事务连接应收到写操作")
	assert.NoError(t, fallbackMock.ExpectationsWereMet(), "fallback 连接不应有任何期望/调用")
}

// TestDBFromContext_FallsBackWhenNoTx 验证：无事务时回退到基础连接。
func TestDBFromContext_FallsBackWhenNoTx(t *testing.T) {
	fallback, fallbackMock := newMockGormDB(t)
	fallbackMock.ExpectExec("UPDATE demo").WillReturnResult(sqlmock.NewResult(0, 1))

	err := DBFromContext(context.Background(), fallback).Exec("UPDATE demo SET x = 1").Error

	require.NoError(t, err)
	assert.NoError(t, fallbackMock.ExpectationsWereMet())
}

// TestDBFromContext_IgnoresLegacyStringKey 验证：类型化 key 与旧的字符串 "tx" key
// 互不冲突——遗留字符串 key 被忽略，操作走 fallback。这是从字符串 key 迁移的关键防回归。
func TestDBFromContext_IgnoresLegacyStringKey(t *testing.T) {
	fallback, fallbackMock := newMockGormDB(t)
	stray, strayMock := newMockGormDB(t)

	// 用旧的字符串 key 塞入一个连接——它绝不能被 DBFromContext 命中。
	ctx := context.WithValue(context.Background(), "tx", stray) //nolint:staticcheck // 故意用字符串 key 测隔离

	fallbackMock.ExpectExec("UPDATE demo").WillReturnResult(sqlmock.NewResult(0, 1))
	err := DBFromContext(ctx, fallback).Exec("UPDATE demo SET x = 1").Error

	require.NoError(t, err)
	assert.NoError(t, fallbackMock.ExpectationsWereMet(), "应走 fallback，忽略字符串 key")
	assert.NoError(t, strayMock.ExpectationsWereMet(), "字符串 key 里的连接不应被触碰")
}

// TestGormTransactionManager_PropagatesTx 端到端验证事务管理器：Begin→写→Commit
// 全程走同一事务连接，fn 内经 DBFromContext 取到的正是该事务。
func TestGormTransactionManager_PropagatesTx(t *testing.T) {
	db, mock := newMockGormDB(t)
	tm := NewTransactionManager(db)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE demo").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := tm.WithTransaction(context.Background(), func(txCtx context.Context) error {
		return DBFromContext(txCtx, db).Exec("UPDATE demo SET x = 1").Error
	})

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestGormTransactionManager_RollsBackOnError 验证：fn 返回错误时回滚，写不落库。
func TestGormTransactionManager_RollsBackOnError(t *testing.T) {
	db, mock := newMockGormDB(t)
	tm := NewTransactionManager(db)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE demo").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	sentinel := context.Canceled
	err := tm.WithTransaction(context.Background(), func(txCtx context.Context) error {
		if e := DBFromContext(txCtx, db).Exec("UPDATE demo SET x = 1").Error; e != nil {
			return e
		}
		return sentinel
	})

	assert.ErrorIs(t, err, sentinel)
	assert.NoError(t, mock.ExpectationsWereMet(), "出错时应回滚")
}
