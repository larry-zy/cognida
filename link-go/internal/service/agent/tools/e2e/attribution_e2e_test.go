//go:build integration

// 归因端到端测试（任务 4a.6）：
// 真实 data_analysis(attribution) → 真实 MCPClient → 真实 Python data_attribution，
// 覆盖 result_id 引用取数、归因信封（drivers 落 Result Store / insight / caliber /
// confidence / drill_down）与 driver ranking 在行序扰动下的稳定性。
//
// 运行前置同 data_analysis_e2e_test.go（Python MCP http server + MCP_TEST_ENDPOINT）。
package e2e

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	agentctx "link/internal/model/agent"
	"link/internal/infrastructure/mcp"
	"link/internal/service/agent/resultstore"
	agenttools "link/internal/service/agent/tools"
)

// attributionRows 两期 × 三区域：华北植入 -400 大幅下降，为确定性头号驱动。
func attributionRows() []map[string]interface{} {
	return []map[string]interface{}{
		{"month": "2026-05", "region": "华北", "gmv": 1000},
		{"month": "2026-05", "region": "华东", "gmv": 800},
		{"month": "2026-05", "region": "华南", "gmv": 500},
		{"month": "2026-06", "region": "华北", "gmv": 600},
		{"month": "2026-06", "region": "华东", "gmv": 850},
		{"month": "2026-06", "region": "华南", "gmv": 500},
	}
}

// attributionE2EEnv 归因 e2e 依赖载体：MCP 调用器与 Result Store 经构造参数
// 显式注入工具（替代旧的包级 Init* 全局注入）。
type attributionE2EEnv struct {
	invoker agenttools.MCPInvoker
	store   resultstore.Store
}

func setupAttributionE2E(t *testing.T) (context.Context, *attributionE2EEnv) {
	t.Helper()
	endpoint := os.Getenv("MCP_TEST_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8899/mcp"
	}
	client, err := mcp.NewMCPClient(&mcp.Config{
		Endpoint: endpoint,
		Timeout:  10 * time.Second,
		CacheTTL: time.Second,
		Logger:   slog.Default(),
	})
	if err != nil {
		t.Fatalf("NewMCPClient: %v", err)
	}
	env := &attributionE2EEnv{invoker: client, store: resultstore.NewMemoryStore()}
	ctx := agentctx.WithSessionID(agentctx.WithTenantID(context.Background(), 1), "sess-e2e-attr")
	return ctx, env
}

// runAttribution 以 result_id 引用取数方式执行一次归因，返回解析后的信封。
func runAttribution(t *testing.T, ctx context.Context, env *attributionE2EEnv, rows []map[string]interface{}) map[string]interface{} {
	t.Helper()
	srcID, err := env.store.Put(ctx, &resultstore.Result{
		Owner:   resultstore.OwnerKey(1, "sess-e2e-attr"),
		Columns: []string{"month", "region", "gmv"},
		Rows:    rows,
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("预置行集失败: %v", err)
	}

	tool, err := agenttools.NewDataAnalysisTool(env.invoker, env.store)
	if err != nil {
		t.Fatalf("NewDataAnalysisTool: %v", err)
	}
	args, _ := json.Marshal(map[string]interface{}{
		"analysis_type": "attribution",
		"result_id":     srcID,
		"options": map[string]interface{}{
			"value_col":  "gmv",
			"period_col": "month",
			"dim_col":    "region",
		},
	})
	out, err := tool.InvokableRun(ctx, string(args))
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if parsed["success"] != true {
		t.Fatalf("attribution not successful: %s", out)
	}
	parsed["__source_result_id"] = srcID
	return parsed
}

// driverSegments 提取信封中 drivers 的切片顺序。
func driverSegments(t *testing.T, envelope map[string]interface{}) []string {
	t.Helper()
	data, _ := envelope["data"].(map[string]interface{})
	drivers, _ := data["drivers"].([]interface{})
	if len(drivers) == 0 {
		t.Fatalf("no drivers in envelope: %v", envelope)
	}
	segs := make([]string, 0, len(drivers))
	for _, d := range drivers {
		m, _ := d.(map[string]interface{})
		seg, _ := m["segment"].(string)
		segs = append(segs, seg)
	}
	return segs
}

// TestAttributionE2E_EnvelopeContract 归因一等信封全链路契约。
func TestAttributionE2E_EnvelopeContract(t *testing.T) {
	ctx, env := setupAttributionE2E(t)
	envelope := runAttribution(t, ctx, env, attributionRows())

	// 头号驱动：华北 -400
	segs := driverSegments(t, envelope)
	if segs[0] != "华北" {
		t.Errorf("top driver = %q, want 华北 (all: %v)", segs[0], segs)
	}

	// drivers 落 Result Store：新 result_id 可取回
	newID, _ := envelope["result_id"].(string)
	if newID == "" {
		t.Fatal("expected drivers result_id in envelope")
	}
	stored, err := env.store.Get(ctx, resultstore.OwnerKey(1, "sess-e2e-attr"), newID)
	if err != nil {
		t.Fatalf("drivers result not retrievable: %v", err)
	}
	if len(stored.Rows) != 3 || stored.Rows[0]["segment"] != "华北" {
		t.Errorf("stored drivers mismatch: %v", stored.Rows)
	}

	// insight / caliber / confidence / drill_down 齐备
	if insight, _ := envelope["insight"].(string); insight == "" {
		t.Error("expected non-empty insight")
	}
	caliber, _ := envelope["caliber"].(map[string]interface{})
	if caliber["method"] != "additive_variance_decomposition" {
		t.Errorf("caliber = %v", envelope["caliber"])
	}
	if conf, _ := envelope["confidence"].(string); conf == "" {
		t.Errorf("confidence missing: %v", envelope["confidence"])
	}
	dd, _ := envelope["drill_down"].(map[string]interface{})
	if dd["source_result_id"] != envelope["__source_result_id"] {
		t.Errorf("drill_down.source_result_id = %v", dd["source_result_id"])
	}
}

// TestAttributionE2E_RankingStableUnderShuffle 行序扰动不改变 driver ranking。
func TestAttributionE2E_RankingStableUnderShuffle(t *testing.T) {
	ctx, env := setupAttributionE2E(t)

	base := attributionRows()
	shuffled := make([]map[string]interface{}, len(base))
	// 确定性重排（倒序），避免测试内随机源
	for i, r := range base {
		shuffled[len(base)-1-i] = r
	}

	segs1 := driverSegments(t, runAttribution(t, ctx, env, base))
	segs2 := driverSegments(t, runAttribution(t, ctx, env, shuffled))
	if len(segs1) != len(segs2) {
		t.Fatalf("driver count mismatch: %v vs %v", segs1, segs2)
	}
	for i := range segs1 {
		if segs1[i] != segs2[i] {
			t.Fatalf("ranking not stable under shuffle: %v vs %v", segs1, segs2)
		}
	}
}
