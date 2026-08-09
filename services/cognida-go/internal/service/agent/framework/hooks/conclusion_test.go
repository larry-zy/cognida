package hooks

import (
	"strings"
	"testing"
)

// respWithToolCall 构造 process/extract 期望的响应形态。
func respWithToolCall(name, output string) map[string]any {
	return map[string]any{
		"tool_calls": []any{
			map[string]any{
				"name":   name,
				"input":  map[string]any{"analysis_type": "insight"},
				"output": output,
			},
		},
	}
}

// TestConclusion_DataAnalysisIsDefaultDataTool 验证 data_analysis 默认即为数据工具（任务 5.1）。
func TestConclusion_DataAnalysisIsDefaultDataTool(t *testing.T) {
	gen := NewConclusionGenerator(nil)
	if !gen.dataTools["data_analysis"] {
		t.Fatal("data_analysis 应默认在 dataTools 中")
	}
	resp := respWithToolCall("data_analysis", `{"success":true}`)
	if !gen.hasDataToolCall(resp) {
		t.Error("含 data_analysis 调用的响应应被识别为数据工具调用")
	}
}

// TestConclusion_NonDataToolIgnored 验证非数据工具不触发结论。
func TestConclusion_NonDataToolIgnored(t *testing.T) {
	gen := NewConclusionGenerator(nil)
	resp := respWithToolCall("web_search", `whatever`)
	if gen.hasDataToolCall(resp) {
		t.Error("web_search 不应被识别为数据工具")
	}
}

// TestConclusion_AnalysisOutputFeedsSummary 验证 data_analysis 输出进入 dataSummary（任务 5.2）。
func TestConclusion_AnalysisOutputFeedsSummary(t *testing.T) {
	gen := NewConclusionGenerator(nil)
	analysisOutput := `{"analysis_type":"insight","success":true,"data":{"recommendations":["扩大投放"]}}`
	resp := respWithToolCall("data_analysis", analysisOutput)

	calls := gen.extractToolCalls(resp)
	if len(calls) != 1 || calls[0].Name != "data_analysis" {
		t.Fatalf("extractToolCalls = %+v", calls)
	}
	if calls[0].Output != analysisOutput {
		t.Errorf("Output 未透传: %q", calls[0].Output)
	}

	summary := gen.buildDataSummary(calls)
	if !strings.Contains(summary, "data_analysis") {
		t.Error("dataSummary 应包含 data_analysis 工具段")
	}
	if !strings.Contains(summary, "扩大投放") {
		t.Errorf("dataSummary 应包含分析输出内容，实际: %s", summary)
	}
}

// TestConclusion_AddDataToolsAdditive 验证默认种子之上仍可追加数据工具。
func TestConclusion_AddDataToolsAdditive(t *testing.T) {
	gen := NewConclusionGenerator(nil).AddDataTools("sql_execute")
	if !gen.dataTools["data_analysis"] {
		t.Error("AddDataTools 不应清除默认的 data_analysis")
	}
	if !gen.dataTools["sql_execute"] {
		t.Error("AddDataTools 应追加 sql_execute")
	}
}
