// Package mysql: 多租户 fail-closed 守卫的单元测试。
// 用 go-sqlmock + DryRun 构造不连真库的 *gorm.DB，验证守卫在"无租户约束"时
// 令读/改/删报错，而在 Scope/手写 tenant_id/显式全局 三种放行路径下不误伤。
package mysql

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// guardTenantModel 含 tenant_id 列——受守卫保护。
type guardTenantModel struct {
	ID       int64
	TenantID int64 `gorm:"column:tenant_id"`
	Name     string
}

func (guardTenantModel) TableName() string { return "guard_tenant_rows" }

// guardGlobalModel 无 tenant_id 列——守卫不应干预。
type guardGlobalModel struct {
	ID   int64
	Name string
}

func (guardGlobalModel) TableName() string { return "guard_global_rows" }

// newGuardedDB 构造一个注册了租户守卫、且以 DryRun 运行的 *gorm.DB。
func newGuardedDB(t *testing.T) *gorm.DB {
	t.Helper()
	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, SkipDefaultTransaction: true})
	require.NoError(t, err)
	require.NoError(t, RegisterTenantGuard(gdb, true))
	return gdb
}

func TestTenantGuard_BlocksUnscopedQueryOnTenantTable(t *testing.T) {
	db := newGuardedDB(t)
	var out []guardTenantModel
	err := db.Find(&out).Error
	assert.ErrorIs(t, err, ErrTenantScopeMissing, "租户表无约束查询必须 fail-closed")
}

func TestTenantGuard_AllowsScopedQuery(t *testing.T) {
	db := newGuardedDB(t)
	var out []guardTenantModel
	err := db.Scopes(TenantScope(7)).Find(&out).Error
	assert.NotErrorIs(t, err, ErrTenantScopeMissing, "施加 TenantScope 后应放行")
}

func TestTenantGuard_AllowsManualTenantWhere(t *testing.T) {
	db := newGuardedDB(t)
	var out []guardTenantModel
	err := db.Where("tenant_id = ?", 7).Find(&out).Error
	assert.NotErrorIs(t, err, ErrTenantScopeMissing, "手写 tenant_id 过滤应被识别为已约束")
}

func TestTenantGuard_AllowsExplicitGlobalScope(t *testing.T) {
	db := newGuardedDB(t)
	var out []guardTenantModel
	err := db.Scopes(GlobalScope()).Find(&out).Error
	assert.NotErrorIs(t, err, ErrTenantScopeMissing, "显式全局 Scope 应放行")
}

func TestTenantGuard_IgnoresNonTenantTable(t *testing.T) {
	db := newGuardedDB(t)
	var out []guardGlobalModel
	err := db.Find(&out).Error
	assert.NotErrorIs(t, err, ErrTenantScopeMissing, "无 tenant_id 列的表不应被守卫干预")
}

func TestTenantGuard_BlocksUnscopedUpdate(t *testing.T) {
	db := newGuardedDB(t)
	err := db.Model(&guardTenantModel{}).Where("id = ?", 1).Update("name", "x").Error
	assert.ErrorIs(t, err, ErrTenantScopeMissing, "租户表无约束更新必须 fail-closed")
}

func TestTenantGuard_BlocksUnscopedDelete(t *testing.T) {
	db := newGuardedDB(t)
	err := db.Where("id = ?", 1).Delete(&guardTenantModel{}).Error
	assert.ErrorIs(t, err, ErrTenantScopeMissing, "租户表无约束删除必须 fail-closed")
}

func TestTenantGuard_DisabledIsNoop(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, SkipDefaultTransaction: true})
	require.NoError(t, err)
	require.NoError(t, RegisterTenantGuard(gdb, false)) // 未启用多租户

	var out []guardTenantModel
	err = gdb.Find(&out).Error
	assert.NotErrorIs(t, err, ErrTenantScopeMissing, "未启用多租户时守卫为空操作")
}
