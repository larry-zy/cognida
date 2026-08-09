//go:build integration

// Package evaluation Text2SQL 评测跨进程 + 真实数据源 e2e 集成测试。
//
// 与 worker_agent_e2e_test.go（纯 computeMetrics 打真实 Python）不同，本用例把
// 真实的只读 SQL 执行器（sqlRunner）接进来，对真实 MySQL 执行「金标准 + 生成」
// 两条 SQL 采集结果集，再走 Go 端 computeMetrics（Type=sql）→ ensureSQLGraders
// 注入 3 个 SQL 评分器 → HTTP → Python compute_sql_metrics → fillMetrics 全链路，
// 验证 sql_exact_match / sql_component_match / sql_execution_accuracy 三指标
// 从「真实结果集」端到端产出，且执行准确率（EX）确实按结果集比对——不等于文本
// 精确匹配（EM）：构造一条「SQL 文本不同但结果集无序相等」的样本让 EX≠EM。
//
// 运行（需先起 Python 评测服务 + 可连 MySQL）：
//
//	COGNIDA_EVAL_E2E_ENDPOINT=http://127.0.0.1:18888 \
//	DB_HOST=localhost DB_PORT=3306 DB_USER=root DB_PASSWORD=... DB_NAME=cognida \
//	  go test -tags=integration -run TestE2E_SQLComputeMetrics_RealDB_Live \
//	  ./internal/service/evaluation/ -v
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	domeval "cognida/internal/model/evaluation"
)

func sqlE2EEnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// TestE2E_SQLComputeMetrics_RealDB_Live 全链路：真实 MySQL 执行 → 真实 Python 服务聚合。
func TestE2E_SQLComputeMetrics_RealDB_Live(t *testing.T) {
	endpoint := os.Getenv("COGNIDA_EVAL_E2E_ENDPOINT")
	if endpoint == "" {
		t.Skip("COGNIDA_EVAL_E2E_ENDPOINT 未设置，跳过跨进程 e2e（需运行中的 Python 评测服务）")
	}

	// 1) 连真实业务库，构造真实只读 SQL 执行器（dsp=nil → 只走业务库路径）。
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		sqlE2EEnvOr("DB_USER", "root"),
		sqlE2EEnvOr("DB_PASSWORD", ""),
		sqlE2EEnvOr("DB_HOST", "localhost"),
		sqlE2EEnvOr("DB_PORT", "3306"),
		sqlE2EEnvOr("DB_NAME", "cognida"),
	)
	gormDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Skipf("无法连接 MySQL（%s），跳过真实数据源 e2e: %v", dsn, err)
	}
	runner := NewSQLRunner(gormDB, nil)

	// 只读校验旁证：写语句必须被真实 runner 拦截（防止评测执行链意外落地写操作）。
	if _, verr := runner.RunReadOnly(context.Background(), "DELETE FROM users WHERE 1=1"); verr == nil {
		t.Fatal("只读校验失效：DELETE 竟被放行")
	}

	// 2) 逐样本：真实执行金标准 + 生成两条 SQL，采集真实结果集。
	//    这里用不依赖具体表数据的字面量查询，保证跨环境确定性。
	//    item0：文本全同     → EM=1, comp=1, EX=1
	//    item1：生成少一行   → EM=0, comp<1, EX=0
	//    item2：换序等价改写 → EM=0, comp=1, EX=1（证明 EX 按结果集无序比对，≠ 文本 EM）
	type sqlCase struct {
		question string
		goldSQL  string
		genSQL   string
	}
	cases := []sqlCase{
		{"取 1 和 2", "SELECT 1 AS n UNION SELECT 2 AS n", "SELECT 1 AS n UNION SELECT 2 AS n"},
		{"取 1 和 2（生成漏了 2）", "SELECT 1 AS n UNION SELECT 2 AS n", "SELECT 1 AS n"},
		{"取 1 和 2（生成换序）", "SELECT 1 AS n UNION SELECT 2 AS n", "SELECT 2 AS n UNION SELECT 1 AS n"},
	}

	ctx := context.Background()
	qaResults := make([]*QAResult, len(cases))
	for i, c := range cases {
		goldRS, gerr := runner.RunReadOnly(ctx, c.goldSQL)
		if gerr != nil {
			t.Fatalf("item%d 金标准 SQL 真实执行失败: %v", i, gerr)
		}
		genRS, xerr := runner.RunReadOnly(ctx, c.genSQL)
		if xerr != nil {
			t.Fatalf("item%d 生成 SQL 真实执行失败: %v", i, xerr)
		}
		qaResults[i] = &QAResult{
			Question:        c.question,
			ReferenceAnswer: c.goldSQL,
			GeneratedAnswer: c.genSQL,
			GoldSQL:         c.goldSQL,
			GeneratedSQL:    c.genSQL,
			GoldResultSet:   goldRS.Rows,
			ResultSet:       genRS.Rows,
			Success:         true,
			LatencyMs:       100 + int64(i)*10,
		}
	}

	// 3) computeMetrics（Type=sql）打真实 Python 服务，全链路聚合三指标。
	config := &DomainEvaluationTaskConfig{DatasetID: "sample_sql", Type: domeval.EvaluationTypeSQL}
	worker := &EvaluationWorker{pythonClient: NewPythonEvaluationClient(endpoint)}
	res, cerr := worker.computeMetrics(ctx, config, qaResults)
	if cerr != nil {
		t.Fatalf("computeMetrics 打真实服务失败: %v", cerr)
	}
	if res.Scores == nil {
		t.Fatal("evalResult.Scores 为空，SQL 聚合指标未回填")
	}

	// 4) 断言三指标齐全且取值符合手算预期。
	//    EM: (1+0+0)/3 = 0.3333；comp: (1 + c1 + 1)/3；EX: (1+0+1)/3 = 0.6667
	assertClose := func(name string, want, tol float64) {
		v, ok := res.Scores[name]
		if !ok {
			t.Errorf("缺少 SQL 聚合指标 %q（Python compute_sql_metrics 未产出或未回填）", name)
			return
		}
		if math.Abs(v-want) > tol {
			t.Errorf("SQL 指标 %q=%v, 期望 ~%v（±%v）", name, v, want, tol)
		}
	}
	assertClose("sql_exact_match", 1.0/3.0, 0.01)
	assertClose("sql_execution_accuracy", 2.0/3.0, 0.01)

	// EX≠EM 是本用例核心：证明执行准确率确实按真实结果集比对而非套用文本匹配。
	em := res.Scores["sql_exact_match"]
	ex := res.Scores["sql_execution_accuracy"]
	if math.Abs(em-ex) < 0.05 {
		t.Errorf("EX(%v) 与 EM(%v) 过于接近，未能证明执行准确率独立按结果集比对", ex, em)
	}

	// component F1 应在 (EM, 1] 之间（换序/漏行都比纯文本更宽容）。
	if c := res.Scores["sql_component_match"]; c <= em || c > 1.0001 {
		t.Errorf("sql_component_match=%v 不在 (%v, 1] 合理区间", c, em)
	}

	// 5) 逐条 Scores 落回：item0 三项全 1，item1 EX=0，item2 EM=0 且 EX=1。
	if len(res.QAResults) != 3 {
		t.Fatalf("期望 3 条逐样本结果，实得 %d", len(res.QAResults))
	}
	if got := res.QAResults[0].Scores["sql_execution_accuracy"]; got != 1.0 {
		t.Errorf("item0 逐条 sql_execution_accuracy=%v, want 1.0", got)
	}
	if got := res.QAResults[1].Scores["sql_execution_accuracy"]; got != 0.0 {
		t.Errorf("item1 逐条 sql_execution_accuracy=%v, want 0.0", got)
	}
	if got := res.QAResults[2].Scores["sql_execution_accuracy"]; got != 1.0 {
		t.Errorf("item2 逐条 sql_execution_accuracy=%v(换序等价应为 1.0)", got)
	}
	if got := res.QAResults[2].Scores["sql_exact_match"]; got != 0.0 {
		t.Errorf("item2 逐条 sql_exact_match=%v(文本不同应为 0.0)", got)
	}

	// 6) 可选落盘：把聚合 + 逐条分值写入 test-output（gitignored），供人工复核。
	if out := os.Getenv("COGNIDA_EVAL_E2E_OUTPUT"); out != "" {
		payload := map[string]any{
			"generated_at": time.Now().Format(time.RFC3339),
			"endpoint":     endpoint,
			"dsn_db":       sqlE2EEnvOr("DB_NAME", "cognida"),
			"aggregate":    res.Scores,
			"per_item": []map[string]float64{
				res.QAResults[0].Scores, res.QAResults[1].Scores, res.QAResults[2].Scores,
			},
		}
		if b, merr := json.MarshalIndent(payload, "", "  "); merr == nil {
			if dir := filepath.Dir(out); dir != "" && dir != "." {
				_ = os.MkdirAll(dir, 0o755)
			}
			if werr := os.WriteFile(out, b, 0o644); werr != nil {
				t.Logf("写入 e2e 结果失败(%s): %v", out, werr)
			} else {
				t.Logf("已写入 e2e 结果: %s", out)
			}
		}
	}

	t.Logf("SQL e2e 全链路通过: EM=%.4f EX=%.4f comp=%.4f",
		res.Scores["sql_exact_match"], res.Scores["sql_execution_accuracy"], res.Scores["sql_component_match"])
}
