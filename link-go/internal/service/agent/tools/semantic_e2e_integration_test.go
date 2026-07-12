//go:build integration

// 指标语义层「治理主路」端到端集成测试：真实 MySQL 验证整条接缝——
//   seed 的语义模型 → semanticQuery 命中治理口径直出 SQL + 数据源绑定 →
//   经真实 ConnectionManager 打向 ecommerce_demo 执行 → 取到真实行 →
//   覆盖埋点落 agent_semantic_coverage_logs 且可按模型聚合读回。
//
// 前置：link 库已 migrate-db 且 cmd/seed-semantic 已灌模（逻辑表绑定电商演示库数据源）、
// cmd/seed-ecommerce 已灌业务数据、data_sources 已登记「电商演示库」、.env 有
// DATASOURCE_SECRET_KEY（凭证解密）。缺任一前置则 t.Skip（不算失败）。
//
// 运行：cd link-go && set -a && source .env && set +a && \
//   go test -tags=integration ./internal/service/agent/tools/ -run E2E -v
package tools

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	agentctx "link/internal/model/agent"
	"link/internal/model/semantic"
	mysqlrepo "link/internal/repository/mysql"
	dssvc "link/internal/service/datasource"
	"link/internal/service/agent/semanticcache"
)

const (
	e2eTenant = int64(1)
	e2eModel  = "电商销售"
)

func openLinkDB(t *testing.T) *gorm.DB {
	t.Helper()
	env := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		env("DB_USER", "root"), env("DB_PASSWORD", ""),
		env("DB_HOST", "127.0.0.1"), env("DB_PORT", "3306"), env("DB_NAME", "link"))
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("link 库不可用，跳过端到端集成测试: %v", err)
	}
	return db
}

// TestSemanticGovernanceE2E 端到端验证治理主路（covered → 打真实库 → 取真实行 → 覆盖可观测）。
func TestSemanticGovernanceE2E(t *testing.T) {
	db := openLinkDB(t)

	repo := mysqlrepo.NewSemanticRepository(db)
	ctx := agentctx.WithRequestID(
		agentctx.WithSessionID(
			agentctx.WithTenantID(context.Background(), e2eTenant),
			"e2e-sess"),
		"e2e-rid")

	// 前置校验：seed 的「电商销售」模型须存在且逻辑表已绑定电商演示库数据源。
	bundle, err := repo.GetActiveModel(ctx, e2eTenant, e2eModel)
	if err != nil {
		t.Skipf("未找到生效语义模型 %q（先跑 cmd/seed-semantic）: %v", e2eModel, err)
	}
	dsID := bundleDatabaseID(bundle)
	require.NotEmpty(t, dsID, "seed 的逻辑表应已绑定电商演示库数据源（跑最新 cmd/seed-semantic）")

	// —— 1) 治理命中：按城市看营收 → covered=true、带 SQL、带数据源绑定 ——
	sink := &recordingCoverageSink{}
	cache := semanticcache.NewMemoryCache()
	res, err := semanticQuery(ctx, &SemanticQueryRequest{
		Model:      e2eModel, // 租户下有多套生效模型，显式指定
		Metrics:    []string{"营收"},
		Dimensions: []string{"城市"},
		Limit:      10,
	}, repo, cache, sink)
	require.NoError(t, err)
	require.True(t, res.Covered, "「按城市看营收」应被语义模型覆盖走治理口径，got %+v", res)
	require.NotEmpty(t, res.SQL, "covered 应产出治理 SQL")
	assert.Equal(t, dsID, res.DatabaseID, "covered 结果应透传模型绑定的数据源 ID")
	assert.Contains(t, res.SQL, "SUM(orders.pay_amount)", "营收口径应钉死为已支付金额之和")
	if ev := sink.last(t); ev.Outcome != semantic.CoverageCovered {
		t.Fatalf("应记 covered 埋点，got %+v", ev)
	}

	// —— 2) 打真实数据源执行治理 SQL → 取到真实行 ——
	cipher, err := dssvc.NewCipherFromEnv()
	require.NoError(t, err, "需 DATASOURCE_SECRET_KEY 以解密数据源凭证")
	cm := dssvc.NewConnectionManager(
		mysqlrepo.NewDataSourceRepository(db), cipher, dssvc.DefaultPoolOptions())

	exec, err := sqlExecute(ctx, &SQLExecuteRequest{
		SQL:        res.SQL,
		DatabaseID: res.DatabaseID, // 关键：显式打向电商演示库，而非会话隐式选库
		MaxRows:    10,
	}, db /*businessDB 兜底，不应命中*/, cm, nil /*无 result store*/)
	require.NoError(t, err, "治理 SQL 应能在 ecommerce_demo 上成功执行")
	require.Greater(t, exec.RowCount, 0, "按城市看营收应取到真实行")
	assert.Contains(t, exec.Columns, "城市")
	assert.Contains(t, exec.Columns, "营收")
	t.Logf("治理直出取到 %d 行（示例列 %v），executed=%s", exec.RowCount, exec.Columns, exec.ExecutedSQL)

	// —— 3) 越界口径 → fallback（+未覆盖名称）——
	fb, err := semanticQuery(ctx, &SemanticQueryRequest{Model: e2eModel, Metrics: []string{"毛利率"}}, repo, cache, sink)
	require.NoError(t, err)
	assert.False(t, fb.Covered, "「毛利率」未建模应回退")
	assert.Contains(t, fb.Uncovered, "毛利率")

	// —— 4) 覆盖可观测：真实覆盖表 Record + 按模型聚合读回，covered≥1 ——
	covRepo := mysqlrepo.NewSemanticCoverageRepository(db)
	require.NoError(t, covRepo.Record(ctx, semantic.CoverageEvent{
		TenantID: e2eTenant, RequestID: "e2e-rid", Model: e2eModel, Outcome: semantic.CoverageCovered,
	}))
	stats, err := covRepo.Stats(ctx, e2eTenant)
	require.NoError(t, err)
	var seen bool
	for _, s := range stats {
		if s.Model == e2eModel {
			seen = true
			assert.GreaterOrEqual(t, s.Covered, int64(1), "电商销售模型应至少 1 次 covered")
			assert.Equal(t, s.Covered+s.CacheHit+s.Fallback, s.Total, "Total 须恒等于三桶之和")
			if s.Total > 0 {
				assert.InDelta(t, float64(s.Covered+s.CacheHit)/float64(s.Total), s.HitRatio, 1e-9)
			}
		}
	}
	assert.True(t, seen, "覆盖聚合应含电商销售模型，got %d models", len(stats))
}
