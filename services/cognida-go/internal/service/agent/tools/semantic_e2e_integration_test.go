//go:build integration

// The semantic governance E2E creates its own business tables, encrypted external
// datasource registration and production semantic model inside MYSQL_DSN's migrated
// test database. It never depends on a manually seeded ecommerce database.
package tools

import (
	modelds "cognida/internal/model/datasource"
	semanticseed "cognida/internal/seed/semantic"
	"context"
	"fmt"
	godriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/semantic"
	mysqlrepo "cognida/internal/repository/mysql"
	"cognida/internal/service/agent/semanticcache"
	dssvc "cognida/internal/service/datasource"
)

const (
	e2eTenant = int64(990018)
)

func openLinkDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	require.NotEmpty(t, dsn, "MYSQL_DSN must identify a migrated integration database")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("link 库不可用: %v", err)
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
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

	prefix := "it_" + uuid.NewString()[:8] + "_"
	ordersTable, customersTable := prefix+"orders", prefix+"customers"
	dsID := prefix + "datasource"
	e2eModel := prefix + "电商销售"
	bundle := semanticseed.SalesBundle(e2eTenant, dsID, prefix)
	bundle.Model.Name = e2eModel
	for _, table := range bundle.LogicalTables {
		if table.Name == "orders" {
			table.PhysicalTable = ordersTable
		} else {
			table.PhysicalTable = customersTable
		}
	}
	t.Cleanup(func() {
		for _, table := range []string{"agent_semantic_dimensions", "agent_semantic_measures", "agent_semantic_metrics", "agent_semantic_relations", "agent_semantic_logical_tables"} {
			require.NoError(t, db.Table(table).Where("model_id = ?", bundle.Model.ID).Delete(map[string]interface{}{}).Error)
		}
		require.NoError(t, db.Where("id = ?", bundle.Model.ID).Delete(&mysqlrepo.SemanticModelModel{}).Error)
		require.NoError(t, db.Where("id = ?", dsID).Delete(&mysqlrepo.DataSourceModel{}).Error)
		require.NoError(t, db.Where("tenant_id = ? AND model = ?", e2eTenant, e2eModel).Delete(&mysqlrepo.SemanticCoverageLogModel{}).Error)
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS `"+ordersTable+"`, `"+customersTable+"`").Error)
	})
	require.NoError(t, db.Exec("CREATE TABLE `"+customersTable+"` (id INT PRIMARY KEY, city VARCHAR(32) NOT NULL)").Error)
	require.NoError(t, db.Exec("CREATE TABLE `"+ordersTable+"` (id INT PRIMARY KEY, customer_id INT NOT NULL, pay_amount DECIMAL(10,2) NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO `"+customersTable+"` VALUES (1, '北京'), (2, '上海')").Error)
	require.NoError(t, db.Exec("INSERT INTO `"+ordersTable+"` VALUES (1, 1, 100), (2, 1, 50), (3, 2, 40)").Error)
	cfg, err := godriver.ParseDSN(os.Getenv("MYSQL_DSN"))
	require.NoError(t, err)
	require.Equal(t, "tcp", cfg.Net, "datasource fixture requires a TCP MySQL connection")
	host, rawPort, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)
	cipher, err := dssvc.NewCipher("semantic-e2e-fixture-secret")
	require.NoError(t, err)
	encrypted, err := cipher.Encrypt(cfg.Passwd)
	require.NoError(t, err)
	dsRepo := mysqlrepo.NewDataSourceRepository(db)
	require.NoError(t, dsRepo.Create(ctx, &modelds.DataSource{
		ID: dsID, TenantID: e2eTenant, Name: dsID, Type: modelds.TypeMySQL,
		Host: host, Port: port, DatabaseName: cfg.DBName, Username: cfg.User,
		PasswordEncrypted: encrypted, Status: modelds.StatusActive, CreatedBy: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))
	require.NoError(t, repo.UpsertBundle(ctx, bundle))
	loaded, err := repo.GetActiveModel(ctx, e2eTenant, e2eModel)
	require.NoError(t, err)
	require.Equal(t, dsID, bundleDatabaseID(loaded))

	// —— 1) 治理命中：按城市看营收 → covered=true、带 SQL、带数据源绑定 ——
	sink := &recordingCoverageSink{}
	cache := semanticcache.NewMemoryCache()
	res, err := semanticQuery(ctx, &SemanticQueryRequest{
		Model:      e2eModel, // 租户下有多套生效模型，显式指定
		Metrics:    []string{"营收"},
		Dimensions: []string{"城市"},
		Limit:      10,
	}, repo, cache, sink, nil)
	require.NoError(t, err)
	require.True(t, res.Covered, "「按城市看营收」应被语义模型覆盖走治理口径，got %+v", res)
	require.NotEmpty(t, res.SQL, "covered 应产出治理 SQL")
	assert.Equal(t, dsID, res.DatabaseID, "covered 结果应透传模型绑定的数据源 ID")
	assert.Contains(t, res.SQL, "SUM(orders.pay_amount)", "营收口径应钉死为已支付金额之和")
	if ev := sink.last(t); ev.Outcome != semantic.CoverageCovered {
		t.Fatalf("应记 covered 埋点，got %+v", ev)
	}

	// —— 2) 打真实数据源执行治理 SQL → 取到真实行 ——
	cm := dssvc.NewConnectionManager(dsRepo, cipher, dssvc.DefaultPoolOptions())
	t.Cleanup(cm.Close)

	exec, err := sqlExecute(ctx, &SQLExecuteRequest{
		SQL:        res.SQL,
		DatabaseID: res.DatabaseID, // 关键：显式打向电商演示库，而非会话隐式选库
		MaxRows:    10,
	}, nil /*禁止回退，必须经外部数据源执行*/, cm, nil /*无 result store*/, nil /*无列画像*/, nil /*无会话注册表*/)
	require.NoError(t, err, "治理 SQL 应能通过外部连接成功执行")
	require.Equal(t, 2, exec.RowCount, "fixture 应恰有两个城市")
	totals := map[string]float64{}
	for _, row := range exec.Samples {
		amount, err := strconv.ParseFloat(fmt.Sprint(row["营收"]), 64)
		require.NoError(t, err)
		totals[fmt.Sprint(row["城市"])] = amount
	}
	require.Equal(t, map[string]float64{"北京": 150, "上海": 40}, totals)
	assert.Contains(t, exec.Columns, "城市")
	assert.Contains(t, exec.Columns, "营收")
	t.Logf("治理直出取到 %d 行（示例列 %v），executed=%s", exec.RowCount, exec.Columns, exec.ExecutedSQL)

	// —— 3) 越界口径 → fallback（+未覆盖名称）——
	fb, err := semanticQuery(ctx, &SemanticQueryRequest{Model: e2eModel, Metrics: []string{"毛利率"}}, repo, cache, sink, nil)
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
