package evaluation

import (
	"testing"

	agentframework "cognida/internal/service/agent/framework"
)

// TestExtractGeneratedSQL_FromInput 验证优先取最后一次 sql_execute 的入参 Input["sql"]。
func TestExtractGeneratedSQL_FromInput(t *testing.T) {
	calls := []*agentframework.ToolCall{
		{Name: "get_schema", Input: map[string]interface{}{}},
		{Name: "sql_execute", Input: map[string]interface{}{"sql": "SELECT 1"}},
		{Name: "sql_execute", Input: map[string]interface{}{"sql": "SELECT COUNT(*) FROM orders"}},
	}
	if got := extractGeneratedSQL(calls); got != "SELECT COUNT(*) FROM orders" {
		t.Errorf("extractGeneratedSQL = %q, want 末次 sql_execute 的 SQL", got)
	}
}

// TestExtractGeneratedSQL_FallbackToOutput 验证入参缺失时回退解析 Output JSON 的 executed_sql。
func TestExtractGeneratedSQL_FallbackToOutput(t *testing.T) {
	calls := []*agentframework.ToolCall{
		{Name: "sql_execute", Input: map[string]interface{}{}, Output: `{"executed_sql":"SELECT 2 LIMIT 100","rows":[]}`},
	}
	if got := extractGeneratedSQL(calls); got != "SELECT 2 LIMIT 100" {
		t.Errorf("extractGeneratedSQL = %q, want 从 Output.executed_sql 回退", got)
	}
}

// TestExtractGeneratedSQL_NoSQLExecute 验证无 sql_execute 调用时返回空串。
func TestExtractGeneratedSQL_NoSQLExecute(t *testing.T) {
	calls := []*agentframework.ToolCall{
		{Name: "get_schema", Input: map[string]interface{}{}},
		{Name: "render_ui", Input: map[string]interface{}{}},
	}
	if got := extractGeneratedSQL(calls); got != "" {
		t.Errorf("extractGeneratedSQL = %q, want empty", got)
	}
}

// TestExtractGeneratedSQL_LastCallEmptyNoBacktrack 验证命中末次 sql_execute 但取不到 SQL 时
// 不再向前回溯（末次即最终答案所依据的查询）。
func TestExtractGeneratedSQL_LastCallEmptyNoBacktrack(t *testing.T) {
	calls := []*agentframework.ToolCall{
		{Name: "sql_execute", Input: map[string]interface{}{"sql": "SELECT 1"}},
		{Name: "sql_execute", Input: map[string]interface{}{}, Output: "not-json"},
	}
	if got := extractGeneratedSQL(calls); got != "" {
		t.Errorf("extractGeneratedSQL = %q, want empty（不回溯到更早的调用）", got)
	}
}
