package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	modeltools "link/internal/model/agent/tools"
)

// mockInvoker 实现 MCPInvoker，用于单测无需真实 MCP server。
type mockInvoker struct {
	gotSkill  string
	gotParams map[string]interface{}
	result    *modeltools.SkillInvokeResult
	err       error
}

func (m *mockInvoker) Invoke(ctx interface{}, skillName string, params map[string]interface{}) (*modeltools.SkillInvokeResult, error) {
	m.gotSkill = skillName
	m.gotParams = params
	return m.result, m.err
}

// withInvoker 临时替换全局注入器，测试后还原。
func withInvoker(t *testing.T, inv MCPInvoker) {
	t.Helper()
	prev := dataAnalysisInvoker
	dataAnalysisInvoker = inv
	t.Cleanup(func() { dataAnalysisInvoker = prev })
}

func TestDataAnalysisTool_Info(t *testing.T) {
	tool, err := NewDataAnalysisTool()
	if err != nil {
		t.Fatalf("NewDataAnalysisTool: %v", err)
	}
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "data_analysis" {
		t.Errorf("name = %q, want data_analysis", info.Name)
	}
}

func TestDataAnalysisTool_Success(t *testing.T) {
	inv := &mockInvoker{
		result: &modeltools.SkillInvokeResult{
			Success: true,
			Data:    map[string]interface{}{"row_count": 6},
		},
	}
	withInvoker(t, inv)

	tool, _ := NewDataAnalysisTool()
	args := `{"analysis_type":"describe","data":{"columns":["a"],"rows":[[1],[2]]},"options":{"columns":["a"]}}`
	out, err := tool.InvokableRun(context.Background(), args)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	// describe → data_describe
	if inv.gotSkill != "data_describe" {
		t.Errorf("skill = %q, want data_describe", inv.gotSkill)
	}
	// options 应被展开进 params，data 透传
	if _, ok := inv.gotParams["data"]; !ok {
		t.Errorf("params missing data: %v", inv.gotParams)
	}
	if cols, ok := inv.gotParams["columns"]; !ok {
		t.Errorf("options not spread into params: %v", inv.gotParams)
	} else {
		_ = cols
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if parsed["success"] != true {
		t.Errorf("success = %v, want true", parsed["success"])
	}
	if parsed["analysis_type"] != "describe" {
		t.Errorf("analysis_type = %v", parsed["analysis_type"])
	}
}

func TestDataAnalysisTool_UnknownType(t *testing.T) {
	withInvoker(t, &mockInvoker{})
	tool, _ := NewDataAnalysisTool()
	args := `{"analysis_type":"bogus","data":{"columns":["a"],"rows":[[1]]}}`
	out, err := tool.InvokableRun(context.Background(), args)
	if err != nil {
		t.Fatalf("InvokableRun should not error on unknown type: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("unknown type should yield success=false, got %v", parsed["success"])
	}
}

func TestDataAnalysisTool_MissingAnalysisType(t *testing.T) {
	withInvoker(t, &mockInvoker{})
	tool, _ := NewDataAnalysisTool()
	_, err := tool.InvokableRun(context.Background(), `{"data":{"columns":["a"],"rows":[[1]]}}`)
	if err == nil {
		t.Error("expected error for missing analysis_type")
	}
}

func TestDataAnalysisTool_MissingData(t *testing.T) {
	withInvoker(t, &mockInvoker{})
	tool, _ := NewDataAnalysisTool()
	_, err := tool.InvokableRun(context.Background(), `{"analysis_type":"describe"}`)
	if err == nil {
		t.Error("expected error for missing data")
	}
}

func TestDataAnalysisTool_NoInvoker(t *testing.T) {
	withInvoker(t, nil)
	tool, _ := NewDataAnalysisTool()
	args := `{"analysis_type":"describe","data":{"columns":["a"],"rows":[[1]]}}`
	out, err := tool.InvokableRun(context.Background(), args)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("no invoker should yield success=false, got %v", parsed["success"])
	}
}

func TestDataAnalysisTool_InvokeError(t *testing.T) {
	withInvoker(t, &mockInvoker{err: errors.New("connection refused")})
	tool, _ := NewDataAnalysisTool()
	args := `{"analysis_type":"trend","data":{"columns":["a"],"rows":[[1]]},"options":{"value_col":"a"}}`
	out, err := tool.InvokableRun(context.Background(), args)
	if err != nil {
		t.Fatalf("InvokableRun should swallow invoke error into result: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("invoke error should yield success=false, got %v", parsed["success"])
	}
	if parsed["error"] == nil {
		t.Error("expected error message in result")
	}
}
