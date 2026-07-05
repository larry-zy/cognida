//go:build integration

// Package e2e 提供 data_analysis 工具的跨栈端到端测试：
// 真实 data_analysis 工具 → 真实 infrastructure/mcp.MCPClient → 真实 Python MCP HTTP server。
//
// 独立成包以避免 tools 包内 integration 测试文件的自引用编译问题
// （被导入时 tools 包的 _test.go 不参与编译）。
//
// 运行前置：启动 Python MCP http server，例如
//
//	MCP_MODE=http MCP_HOST=127.0.0.1 MCP_PORT=8899 .venv/bin/python -m mcp_service.server
//
// 端点可经 MCP_TEST_ENDPOINT 覆盖（默认 http://127.0.0.1:8899/mcp）。
package e2e

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"link/internal/infrastructure/mcp"
	agenttools "link/internal/service/agent/tools"
)

func TestDataAnalysisE2E_RealMCP(t *testing.T) {
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
	// MCP 调用器经构造参数显式注入（替代旧的包级 Init* 全局注入）；
	// 本用例走内联 data 传参，无需 Result Store。
	tool, err := agenttools.NewDataAnalysisTool(client, nil)
	if err != nil {
		t.Fatalf("NewDataAnalysisTool: %v", err)
	}

	rowset := map[string]any{
		"columns": []any{"month", "sales", "cost"},
		"rows": []any{
			[]any{"2024-01", 100, 80},
			[]any{"2024-02", 120, 85},
			[]any{"2024-03", 150, 90},
			[]any{"2024-04", 170, 95},
			[]any{"2024-05", 999, 100},
			[]any{"2024-06", 210, 110},
		},
	}

	cases := []struct {
		analysisType string
		options      map[string]any
		wantKey      string
	}{
		{"describe", nil, "row_count"},
		{"trend", map[string]any{"value_col": "sales"}, "trend"},
		{"anomaly", map[string]any{"value_col": "sales"}, "anomaly_count"},
		{"correlation", nil, "correlation"},
		{"insight", nil, "recommendations"},
	}

	for _, tc := range cases {
		t.Run(tc.analysisType, func(t *testing.T) {
			args := map[string]any{"analysis_type": tc.analysisType, "data": rowset}
			if tc.options != nil {
				args["options"] = tc.options
			}
			argsJSON, _ := json.Marshal(args)

			out, err := tool.InvokableRun(context.Background(), string(argsJSON))
			if err != nil {
				t.Fatalf("InvokableRun: %v", err)
			}

			var parsed map[string]any
			if err := json.Unmarshal([]byte(out), &parsed); err != nil {
				t.Fatalf("output not JSON: %v\n%s", err, out)
			}
			if parsed["success"] != true {
				t.Fatalf("analysis %s not successful: %s", tc.analysisType, out)
			}
			data, ok := parsed["data"].(map[string]any)
			if !ok {
				t.Fatalf("missing data envelope: %s", out)
			}
			if _, ok := data[tc.wantKey]; !ok {
				t.Errorf("%s result missing key %q: %v", tc.analysisType, tc.wantKey, data)
			}
			t.Logf("%s ✓ success, keys=%v", tc.analysisType, mapKeys(data))
		})
	}
}

func mapKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
