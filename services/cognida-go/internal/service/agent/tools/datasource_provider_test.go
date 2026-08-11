// Package tools 测试查询类工具的数据源路由（task 3.3/3.4/3.5）。
package tools

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	agentctx "cognida/internal/model/agent"
	model_datasource "cognida/internal/model/datasource"
)

// fakeConnProvider 测试用连接提供者：仅识别一个数据源 ID，其余一律报错（模拟无效 id）。
type fakeConnProvider struct {
	id     string
	dbName string
	db     *sql.DB
}

func (f *fakeConnProvider) Acquire(_ context.Context, _ int64, datasourceID string) (*sql.DB, *model_datasource.DataSource, error) {
	if datasourceID != f.id {
		return nil, nil, fmt.Errorf("数据源不存在或无权访问")
	}
	return f.db, &model_datasource.DataSource{ID: f.id, DatabaseName: f.dbName}, nil
}

// TestResolveQueryTargetProviderMissing database_id 非空但提供者未注入 → 显式报错，不回落业务库。
func TestResolveQueryTargetProviderMissing(t *testing.T) {
	_, err := resolveQueryTarget(context.Background(), "ds-x", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "外部数据源功能未启用") {
		t.Fatalf("提供者未注入应显式报错, got %v", err)
	}
}

// TestResolveQueryTargetInvalidIDNoFallback 无效 database_id → 显式报错，绝不静默回落业务库。
func TestResolveQueryTargetInvalidIDNoFallback(t *testing.T) {
	gormDB, _ := setupMockSchemaDB(t)
	rawDB, _ := gormDB.DB()
	dsp := &fakeConnProvider{id: "ds-ok", dbName: "extdb", db: rawDB}

	// businessDB 传入非 nil：若实现里错误回落业务库，这里将不会报错 → 测试失败暴露问题。
	_, err := resolveQueryTarget(context.Background(), "ds-missing", gormDB, dsp)
	if err == nil || !strings.Contains(err.Error(), "不可用") {
		t.Fatalf("无效数据源 ID 必须显式报错且不回落业务库, got %v", err)
	}
}

// TestResolveQueryTargetSessionDefault 请求未传 database_id 时以会话上下文 datasource_id 兜底。
func TestResolveQueryTargetSessionDefault(t *testing.T) {
	gormDB, _ := setupMockSchemaDB(t)
	rawDB, _ := gormDB.DB()
	dsp := &fakeConnProvider{id: "ds-session", dbName: "extdb", db: rawDB}

	ctx := agentctx.WithDatasourceID(context.Background(), "ds-session")
	target, err := resolveQueryTarget(ctx, "", nil, dsp)
	if err != nil {
		t.Fatalf("会话数据源兜底失败: %v", err)
	}
	if !target.external || target.dbName != "extdb" {
		t.Fatalf("应路由到会话数据源 extdb, got %+v", target)
	}
}

// TestResolveQueryTargetSessionOverridesToolParam 会话已手动选库时以其为准，覆盖工具入参
// database_id（手动选库权威）：模型自选的 database_id 不得覆盖用户的手动选择。
func TestResolveQueryTargetSessionOverridesToolParam(t *testing.T) {
	gormDB, _ := setupMockSchemaDB(t)
	rawDB, _ := gormDB.DB()
	// 提供者只认会话锁定的 ds-manual；若实现错误采纳了工具入参 ds-agent，Acquire 会报错 → 测试失败。
	dsp := &fakeConnProvider{id: "ds-manual", dbName: "manualdb", db: rawDB}

	ctx := agentctx.WithDatasourceID(context.Background(), "ds-manual")
	target, err := resolveQueryTarget(ctx, "ds-agent", gormDB, dsp)
	if err != nil {
		t.Fatalf("手动选库应权威覆盖工具入参, got err %v", err)
	}
	if !target.external || target.dbName != "manualdb" {
		t.Fatalf("应路由到会话手动锁定的 manualdb（非工具入参 ds-agent）, got %+v", target)
	}
}

// TestResolveQueryTargetBusinessBackwardCompat database_id 与会话数据源均为空 → 业务库（向后兼容）。
func TestResolveQueryTargetBusinessBackwardCompat(t *testing.T) {
	gormDB, _ := setupMockSchemaDB(t)

	target, err := resolveQueryTarget(context.Background(), "", gormDB, nil)
	if err != nil {
		t.Fatalf("空 database_id 应走业务库: %v", err)
	}
	if target.external {
		t.Fatal("空 database_id 不应标记为外部数据源")
	}

	// 业务库未初始化 → 显式报错
	if _, err := resolveQueryTarget(context.Background(), "", nil, nil); err == nil {
		t.Fatal("业务库未初始化应报错")
	}
}

// TestSQLMutateBlockedOnExternalDatasource 外部数据源会话屏蔽 sql_mutate（只读红线）。
func TestSQLMutateBlockedOnExternalDatasource(t *testing.T) {
	o := &opTools{cfg: OperationConfig{DangerRowThreshold: DefaultDangerRowThreshold}}
	ctx := agentctx.WithDatasourceID(context.Background(), "ds-ext")
	_, err := o.sqlMutate(ctx, &SQLMutateRequest{SQL: "UPDATE t SET a = 1", IdempotencyKey: "k1"})
	if err == nil || !strings.Contains(err.Error(), "只读") {
		t.Fatalf("外部数据源会话必须拒绝 sql_mutate, got %v", err)
	}
}

// TestETLRunBlockedOnExternalDatasource 外部数据源会话屏蔽 etl_run。
func TestETLRunBlockedOnExternalDatasource(t *testing.T) {
	o := &opTools{cfg: OperationConfig{DangerRowThreshold: DefaultDangerRowThreshold}}
	ctx := agentctx.WithDatasourceID(context.Background(), "ds-ext")
	_, err := o.etlRun(ctx, &ETLRunRequest{TargetTable: "agent_etl_x", SQL: "SELECT 1"})
	if err == nil || !strings.Contains(err.Error(), "只读") {
		t.Fatalf("外部数据源会话必须拒绝 etl_run, got %v", err)
	}
}
