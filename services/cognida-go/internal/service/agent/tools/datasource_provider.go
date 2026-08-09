// Package tools 提供查询类工具的数据源路由（业务库 / 已注册外部数据源）。
package tools

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/gorm"

	agentctx "cognida/internal/model/agent"
	model_datasource "cognida/internal/model/datasource"
)

// queryTarget 查询类工具的执行目标：统一为 database/sql 的 *sql.DB。
// 业务库经 gorm.DB.DB() 取底层连接池；外部数据源经 ConnectionManager 取受管池。
// db 由各自的池管理，调用方不得 Close。
type queryTarget struct {
	db       *sql.DB
	dbName   string   // 外部数据源注册时已知；业务库经 databaseName() 惰性解析
	gormDB   *gorm.DB // 业务库路径持有，供惰性解析库名
	external bool     // 是否外部数据源（错误脱敏与提示语用）
}

// databaseName 返回目标库名（information_schema 过滤用）。
// 业务库首次调用发一次 SELECT DATABASE()；外部数据源直接用注册配置，不发额外查询。
func (t *queryTarget) databaseName() (string, error) {
	if t.dbName != "" {
		return t.dbName, nil
	}
	name := t.gormDB.Migrator().CurrentDatabase()
	if name == "" {
		return "", fmt.Errorf("无法解析当前数据库名")
	}
	t.dbName = name
	return t.dbName, nil
}

// databaseNameContext 同 databaseName，但把惰性解析的 SELECT DATABASE() 纳入传入 ctx 的超时预算。
// 失败线索检索路径复用其短超时上下文（hintQueryTTL），避免 DB 挂起时库名解析无界阻塞。
func (t *queryTarget) databaseNameContext(ctx context.Context) (string, error) {
	if t.dbName != "" {
		return t.dbName, nil
	}
	name := t.gormDB.WithContext(ctx).Migrator().CurrentDatabase()
	if name == "" {
		return "", fmt.Errorf("无法解析当前数据库名")
	}
	t.dbName = name
	return t.dbName, nil
}

// resolveQueryTarget 解析查询目标：
//   - databaseID 为空时先以会话上下文的 datasource_id 兜底（会话级默认数据源）；
//   - 最终仍为空 → businessDB 当前业务库（向后兼容）；
//   - 非空 → 视为已注册外部数据源 ID，经 ConnectionProvider 按租户路由；
//     无效 id / 提供者未注入(dsp==nil) → 显式错误，绝不静默回落业务库。
func resolveQueryTarget(ctx context.Context, databaseID string, businessDB *gorm.DB, dsp model_datasource.ConnectionProvider) (*queryTarget, error) {
	if databaseID == "" {
		databaseID = agentctx.MustGetDatasourceID(ctx)
	}

	// 业务库路径
	if databaseID == "" {
		if businessDB == nil {
			return nil, fmt.Errorf("数据库未初始化")
		}
		db, err := businessDB.DB()
		if err != nil {
			return nil, fmt.Errorf("获取数据库连接失败: %w", err)
		}
		return &queryTarget{db: db, gormDB: businessDB}, nil
	}

	// 外部数据源路径
	if dsp == nil {
		return nil, fmt.Errorf("外部数据源功能未启用，无法使用 database_id=%q（请留空以查询当前业务库）", databaseID)
	}
	tenantID := agentctx.MustGetTenantID(ctx)
	db, ds, err := dsp.Acquire(ctx, tenantID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("数据源 %q 不可用: %w", databaseID, err)
	}
	return &queryTarget{db: db, dbName: ds.DatabaseName, external: true}, nil
}
