// Package mysql: 知识库仓储租户隔离单元测试（sqlmock 断言 SQL 层 tenant_id 谓词）。
package mysql

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	gorm_mysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newMockDB 造一个基于 sqlmock 的 gorm.DB。
func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(gorm_mysql.New(gorm_mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return db, mock
}

// TestFindByIDForTenant_SQLHasTenantPredicate 查询 SQL 必须带 tenant_id 谓词（纵深防御），
// 跨租户命中 0 行时返回"知识库不存在"。
func TestFindByIDForTenant_SQLHasTenantPredicate(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewKnowledgeBaseRepository(db, true)

	// 精确断言 WHERE id=? AND tenant_id=?；返回 0 行模拟跨租户访问（第三个参数是 First 的 LIMIT）
	mock.ExpectQuery(regexp.QuoteMeta("WHERE id = ? AND tenant_id = ?")).
		WithArgs("kb-b", int64(1), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := repo.FindByIDForTenant(context.Background(), "kb-b", 1)
	if err == nil {
		t.Fatal("跨租户查询命中 0 行时应返回错误")
	}
	if err.Error() != "知识库不存在" {
		t.Errorf("错误信息 = %q, 期望与不存在一致的\"知识库不存在\"", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 断言未满足（tenant_id 谓词缺失？）: %v", err)
	}
}

// TestFindByIDForTenant_MatchReturnsKB id+tenant 双键命中时正常返回。
func TestFindByIDForTenant_MatchReturnsKB(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewKnowledgeBaseRepository(db, true)

	mock.ExpectQuery(regexp.QuoteMeta("WHERE id = ? AND tenant_id = ?")).
		WithArgs("kb-a", int64(1), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name"}).
			AddRow("kb-a", int64(1), "我的知识库"))

	kb, err := repo.FindByIDForTenant(context.Background(), "kb-a", 1)
	if err != nil {
		t.Fatalf("归属租户查询应成功: %v", err)
	}
	if kb.ID != "kb-a" || kb.TenantID != 1 {
		t.Errorf("kb = {ID:%s TenantID:%d}, 期望 {kb-a 1}", kb.ID, kb.TenantID)
	}
}

// TestFindByIDForTenant_InvalidTenant_NoQuery tenantID 非法时直接拒绝，不发 SQL（0 行受影响）。
func TestFindByIDForTenant_InvalidTenant_NoQuery(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewKnowledgeBaseRepository(db, true)

	// 不设任何 Expect：若发出 SQL 则 ExpectationsWereMet 会报"未预期的查询"
	if _, err := repo.FindByIDForTenant(context.Background(), "kb-b", 0); err == nil {
		t.Error("tenantID=0 应被拒绝")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("非法 tenantID 不应发出任何 SQL: %v", err)
	}
}
