package chat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// realDSMLBlock 是从线上会话原样抓取的 DeepSeek 原生工具调用块（全角竖线 U+FF5C）。
// 前面带一段正常的思考正文，模拟"标记内联在 content 里"的真实泄漏场景。
const realDSMLContent = "数据已相当完整。现在用分析引擎对核心数据做趋势与洞察分析。\n\n" +
	"<｜｜DSML｜｜tool_calls>\n" +
	"<｜｜DSML｜｜invoke name=\"data_analysis\">\n" +
	"<｜｜DSML｜｜parameter name=\"analysis_type\" string=\"true\">report</｜｜DSML｜｜parameter>\n" +
	"<｜｜DSML｜｜parameter name=\"result_id\" string=\"true\">rs_a03a3480</｜｜DSML｜｜parameter>\n" +
	"<｜｜DSML｜｜parameter name=\"insights\" string=\"false\">[\"G\",\"A\"]</｜｜DSML｜｜parameter>\n" +
	"</｜｜DSML｜｜invoke>\n" +
	"</｜｜DSML｜｜tool_calls>"

func TestParseDSMLToolCalls_RealCapture(t *testing.T) {
	cleaned, calls, found := parseDSMLToolCalls(realDSMLContent)
	if !found {
		t.Fatalf("expected DSML tool calls to be found")
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}

	tc := calls[0]
	if tc.Function.Name != "data_analysis" {
		t.Errorf("name = %q, want data_analysis", tc.Function.Name)
	}
	if tc.ID == "" {
		t.Errorf("expected a synthetic tool call ID, got empty")
	}
	if tc.Type != "function" {
		t.Errorf("type = %q, want function", tc.Type)
	}

	// Arguments 必须是合法 JSON，且类型正确：string="true" → 字符串；string="false" → 原生 JSON。
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Fatalf("arguments not valid JSON: %v (%s)", err, tc.Function.Arguments)
	}
	if args["analysis_type"] != "report" {
		t.Errorf("analysis_type = %v, want report", args["analysis_type"])
	}
	if args["result_id"] != "rs_a03a3480" {
		t.Errorf("result_id = %v, want rs_a03a3480", args["result_id"])
	}
	insights, ok := args["insights"].([]interface{})
	if !ok || !reflect.DeepEqual(insights, []interface{}{"G", "A"}) {
		t.Errorf("insights = %#v, want []interface{}{\"G\",\"A\"}", args["insights"])
	}

	// 用户可见正文里不得残留任何 DSML 标记。
	if strings.Contains(cleaned, "DSML") {
		t.Errorf("cleaned content still contains DSML markers: %q", cleaned)
	}
	if !strings.Contains(cleaned, "现在用分析引擎") {
		t.Errorf("cleaned content dropped legitimate prose: %q", cleaned)
	}
}

func TestParseDSMLToolCalls_NoMarkersUnchanged(t *testing.T) {
	plain := "这是一段普通回答，包含 GMV=125 万等结论，但没有任何工具调用。"
	cleaned, calls, found := parseDSMLToolCalls(plain)
	if found {
		t.Errorf("did not expect tool calls in plain text")
	}
	if len(calls) != 0 {
		t.Errorf("expected no calls, got %d", len(calls))
	}
	if cleaned != plain {
		t.Errorf("plain content should be returned unchanged, got %q", cleaned)
	}
}

func TestParseDSMLToolCalls_MultipleInvokes(t *testing.T) {
	content := "<｜｜DSML｜｜tool_calls>" +
		"<｜｜DSML｜｜invoke name=\"sql_execute\"><｜｜DSML｜｜parameter name=\"sql\" string=\"true\">SELECT 1</｜｜DSML｜｜parameter></｜｜DSML｜｜invoke>" +
		"<｜｜DSML｜｜invoke name=\"data_analysis\"><｜｜DSML｜｜parameter name=\"result_id\" string=\"true\">rs_1</｜｜DSML｜｜parameter></｜｜DSML｜｜invoke>" +
		"</｜｜DSML｜｜tool_calls>"
	_, calls, found := parseDSMLToolCalls(content)
	if !found || len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d (found=%v)", len(calls), found)
	}
	if calls[0].Function.Name != "sql_execute" || calls[1].Function.Name != "data_analysis" {
		t.Errorf("unexpected call order: %s, %s", calls[0].Function.Name, calls[1].Function.Name)
	}
	if calls[0].ID == calls[1].ID {
		t.Errorf("expected distinct synthetic IDs, both = %q", calls[0].ID)
	}
}

// TestDSMLFilter_WholeChunk：整块 content 一次到达时，过滤器剥离标记、抽取工具调用。
func TestDSMLFilter_WholeChunk(t *testing.T) {
	f := &dsmlFilter{}
	emit := f.feed(realDSMLContent)
	tail, calls := f.finish()
	visible := emit + tail

	if strings.Contains(visible, "DSML") {
		t.Errorf("visible text leaked DSML markers: %q", visible)
	}
	if !strings.Contains(visible, "现在用分析引擎") {
		t.Errorf("visible text dropped prose: %q", visible)
	}
	if len(calls) != 1 || calls[0].Function.Name != "data_analysis" {
		t.Fatalf("expected 1 data_analysis call, got %d", len(calls))
	}
}

// TestDSMLFilter_SplitAcrossChunks：把整段内容按每 3 字节切片喂入，
// 标记会被切在任意（含多字节字符中间的）边界上——过滤器仍须不泄露标记且正确抽取调用。
func TestDSMLFilter_SplitAcrossChunks(t *testing.T) {
	f := &dsmlFilter{}
	var visible strings.Builder
	b := []byte(realDSMLContent)
	for i := 0; i < len(b); i += 3 {
		end := i + 3
		if end > len(b) {
			end = len(b)
		}
		visible.WriteString(f.feed(string(b[i:end])))
	}
	tail, calls := f.finish()
	visible.WriteString(tail)

	got := visible.String()
	if strings.Contains(got, "DSML") {
		t.Errorf("split-chunk visible text leaked DSML markers: %q", got)
	}
	if strings.Contains(got, "tool_calls") {
		t.Errorf("split-chunk visible text leaked tool_calls marker: %q", got)
	}
	if !strings.Contains(got, "现在用分析引擎") {
		t.Errorf("split-chunk visible text dropped prose: %q", got)
	}
	if len(calls) != 1 || calls[0].Function.Name != "data_analysis" {
		t.Fatalf("split-chunk: expected 1 data_analysis call, got %d", len(calls))
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("split-chunk arguments invalid JSON: %v", err)
	}
	if args["analysis_type"] != "report" {
		t.Errorf("split-chunk analysis_type = %v, want report", args["analysis_type"])
	}
}

// TestDSMLFilter_PlainTextStreamsThrough：无标记的普通文本必须原样、完整地流出。
func TestDSMLFilter_PlainTextStreamsThrough(t *testing.T) {
	f := &dsmlFilter{}
	chunks := []string{"这是", "一段普通", "回答 < 带个尖括号 ", "但不是标记"}
	var visible strings.Builder
	for _, c := range chunks {
		visible.WriteString(f.feed(c))
	}
	tail, calls := f.finish()
	visible.WriteString(tail)

	want := strings.Join(chunks, "")
	if visible.String() != want {
		t.Errorf("plain stream altered: got %q, want %q", visible.String(), want)
	}
	if len(calls) != 0 {
		t.Errorf("expected no tool calls in plain stream, got %d", len(calls))
	}
}

// TestDSMLFilter_ProseAfterBlock：标记块之后若还有正文，应在块解析后继续下发。
func TestDSMLFilter_ProseAfterBlock(t *testing.T) {
	content := realDSMLContent + "\n\n分析已提交，请稍候。"
	f := &dsmlFilter{}
	emit := f.feed(content)
	tail, calls := f.finish()
	visible := emit + tail

	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if !strings.Contains(visible, "分析已提交，请稍候。") {
		t.Errorf("prose after block was dropped: %q", visible)
	}
	if strings.Contains(visible, "DSML") {
		t.Errorf("visible leaked markers: %q", visible)
	}
}

func TestHoldbackLen(t *testing.T) {
	marker := dsmlOpenMarker
	// 尾部正好是标记前 4 字节 "<｜｜"（"<" + 一个全角竖线 = 1+3 字节起…）——需暂留。
	partial := "正文" + marker[:5]
	if got := holdbackLen(partial, marker); got != 5 {
		t.Errorf("holdbackLen partial = %d, want 5", got)
	}
	// 完全无关的尾部，不暂留。
	if got := holdbackLen("正常结尾。", marker); got != 0 {
		t.Errorf("holdbackLen unrelated = %d, want 0", got)
	}
}
