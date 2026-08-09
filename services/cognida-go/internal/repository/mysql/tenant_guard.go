package mysql

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// ========================================
// 多租户 fail-closed 兜底守卫
// ========================================
//
// 背景：租户隔离原本"选择性"——全靠每个 handler/repo 记得调用 WithTenantScope。
// 一次遗忘即跨租户数据泄漏（读）或跨租户改删（写）。本守卫在启用多租户
// (TENANT_ENABLED=true) 时注册全局 GORM 回调：对任何包含 tenant_id 列的表，
// 若一次 Query/Row/Update/Delete 未经任何租户约束（既无 Scope 标记、WHERE 中也未
// 提及 tenant_id），则直接令该操作报错——fail-closed，宁可报错也不越权。
//
// 合法的跨租户/系统级操作必须显式声明：使用 GlobalScope() / WithGlobalScope()。

const (
	// tenantScopeSettingKey 由租户 Scope 辅助函数写入，标记该次操作已受租户约束。
	tenantScopeSettingKey = "cognida:tenant_scoped"
	// tenantColumn 业务表统一的租户列名。
	tenantColumn = "tenant_id"
)

// ErrTenantScopeMissing 在多租户模式下对租户表执行了无租户约束的操作。
var ErrTenantScopeMissing = errors.New("mysql: 缺少租户 Scope，拒绝对租户表执行无过滤操作（fail-closed）")

// markTenantScoped 在当前 DB 会话上打标记：调用方已显式施加租户约束（或显式全局）。
func markTenantScoped(db *gorm.DB) *gorm.DB {
	return db.Set(tenantScopeSettingKey, true)
}

// RegisterTenantGuard 注册多租户 fail-closed 守卫回调。enabled=false 时为空操作，
// 因此默认单租户部署（TENANT_ENABLED 未开）行为完全不变。
func RegisterTenantGuard(db *gorm.DB, enabled bool) error {
	if db == nil || !enabled {
		return nil
	}
	cb := db.Callback()
	// 注册在各操作真正执行之前，Scope 已在此前被 executeScopes 展开。
	if err := cb.Query().Before("gorm:query").Register("cognida:tenant_guard_query", guard); err != nil {
		return err
	}
	if err := cb.Row().Before("gorm:row").Register("cognida:tenant_guard_row", guard); err != nil {
		return err
	}
	if err := cb.Update().Before("gorm:update").Register("cognida:tenant_guard_update", guard); err != nil {
		return err
	}
	if err := cb.Delete().Before("gorm:delete").Register("cognida:tenant_guard_delete", guard); err != nil {
		return err
	}
	return nil
}

// guard 是守卫回调本体。
func guard(db *gorm.DB) {
	if db.Statement == nil || db.Error != nil {
		return
	}
	// 原生 SQL / 无 schema 的操作不拦截（无法可靠判断表结构）。
	if db.Statement.Schema == nil {
		return
	}
	if !schemaHasTenantColumn(db.Statement.Schema) {
		return
	}
	// 已被 Scope 辅助函数显式标记（含显式全局）——放行。
	if v, ok := db.Statement.Settings.Load(tenantScopeSettingKey); ok && v == true {
		return
	}
	// 兜底：WHERE 中显式提及 tenant_id 也视为已约束（覆盖手写 .Where("tenant_id = ?")）。
	if whereMentionsTenant(db.Statement) {
		return
	}
	_ = db.AddError(ErrTenantScopeMissing)
}

// schemaHasTenantColumn 判断 model 是否含 tenant_id 列。
func schemaHasTenantColumn(s *schema.Schema) bool {
	if s == nil {
		return false
	}
	if f, ok := s.FieldsByDBName[tenantColumn]; ok && f != nil {
		return true
	}
	return false
}

// whereMentionsTenant 尽力检测 WHERE 子句是否对 tenant_id 施加了约束。
func whereMentionsTenant(stmt *gorm.Statement) bool {
	c, ok := stmt.Clauses["WHERE"]
	if !ok {
		return false
	}
	where, ok := c.Expression.(clause.Where)
	if !ok {
		return false
	}
	return exprsMentionTenant(where.Exprs)
}

func exprsMentionTenant(exprs []clause.Expression) bool {
	for _, e := range exprs {
		switch v := e.(type) {
		case clause.Expr:
			if strings.Contains(v.SQL, tenantColumn) {
				return true
			}
		case clause.Eq:
			if columnName(v.Column) == tenantColumn {
				return true
			}
		case clause.IN:
			if columnName(v.Column) == tenantColumn {
				return true
			}
		case clause.AndConditions:
			if exprsMentionTenant(v.Exprs) {
				return true
			}
		}
	}
	return false
}

// columnName 从 clause.Column / string 中提取列名。
func columnName(col interface{}) string {
	switch c := col.(type) {
	case clause.Column:
		return c.Name
	case string:
		return c
	default:
		return ""
	}
}
