package prompt

import (
	"strings"
	"testing"
)

// wantKeys 列出每个业务 YAML 期望存在的 key。调用点用 MustGet 取用，
// 缺失即在启动时 panic；此表把「契约」显式化，防止改配置时误删 key。
var wantKeys = map[string][]string{
	"data_agent": {
		"system",
		"playbook_fetch", "playbook_trend", "playbook_attribution",
		"playbook_report", "playbook_general", "playbook_ambiguous",
	},
	"genui":         {"system"},
	"experience":    {"summarize_system"},
	"collaboration": {"decompose_system", "factcheck_system", "synthesis_system", "merge_consensus_system", "conflict_resolution_system"},
	"knowledge":     {"hyde_system_base", "rewrite_system", "query_expand", "query_decompose", "reasoning_system", "final_answer_system"},
	"evaluation":    {"rag_system_no_context", "rag_system_with_context", "qa_system"},
}

// TestExpectedKeysPresent 保证契约中的每个 key 均可取到非空正文。
func TestExpectedKeysPresent(t *testing.T) {
	for business, keys := range wantKeys {
		for _, k := range keys {
			v, ok := Get(business, k)
			if !ok {
				t.Errorf("缺少提示词 %s/%s", business, k)
				continue
			}
			if strings.TrimSpace(v) == "" {
				t.Errorf("提示词 %s/%s 为空", business, k)
			}
		}
	}
}

// TestNoTrailingNewline 保证所有正文均无尾随换行——迁移前的 Go 原始字符串
// 常量均以非换行符结尾，YAML 用 |- 块标量保持一致，避免拼接提示词时多出空行。
func TestNoTrailingNewline(t *testing.T) {
	for business := range wantKeys {
		for _, k := range Keys(business) {
			v := MustGet(business, k)
			if strings.HasSuffix(v, "\n") {
				t.Errorf("提示词 %s/%s 存在尾随换行", business, k)
			}
		}
	}
}

// TestFmtPlaceholders 保证含 fmt.Sprintf 占位符的模板占位符数量不被误改。
func TestFmtPlaceholders(t *testing.T) {
	cases := []struct {
		business, key string
		wantVerbs     int
	}{
		{"knowledge", "query_expand", 2},               // %d, %s
		{"knowledge", "query_decompose", 1},            // %s
		{"evaluation", "rag_system_with_context", 1},   // %s
	}
	for _, c := range cases {
		v := MustGet(c.business, c.key)
		got := strings.Count(v, "%d") + strings.Count(v, "%s")
		if got != c.wantVerbs {
			t.Errorf("%s/%s 占位符数量=%d，期望 %d", c.business, c.key, got, c.wantVerbs)
		}
	}
}
