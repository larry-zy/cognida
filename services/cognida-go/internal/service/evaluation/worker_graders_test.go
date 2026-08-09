package evaluation

import (
	"sort"
	"testing"
)

// TestEnsureSQLGradersStripsGenerationGraders 验证 SQL 评测剔除 rouge/bleu 生成类评分器
// （对 SQL 无意义、污染详情页），并幂等注入 sql_* 家族。
func TestEnsureSQLGraders(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "默认 rouge/bleu 被剔除并注入 sql 家族",
			in:   []string{"rouge", "bleu"},
			want: []string{"sql_component_match", "sql_exact_match", "sql_execution_accuracy"},
		},
		{
			name: "细粒度生成评分器也被剔除",
			in:   []string{"rouge_1", "rouge_l", "bleu_4"},
			want: []string{"sql_component_match", "sql_exact_match", "sql_execution_accuracy"},
		},
		{
			name: "保留非生成类评分器，去重且幂等",
			in:   []string{"exact_match", "sql_exact_match", "rouge", "exact_match"},
			want: []string{"exact_match", "sql_component_match", "sql_exact_match", "sql_execution_accuracy"},
		},
		{
			name: "空输入仅注入 sql 家族",
			in:   nil,
			want: []string{"sql_component_match", "sql_exact_match", "sql_execution_accuracy"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ensureSQLGraders(c.in)
			gs := append([]string(nil), got...)
			sort.Strings(gs)
			ws := append([]string(nil), c.want...)
			sort.Strings(ws)
			if len(gs) != len(ws) {
				t.Fatalf("ensureSQLGraders(%v)=%v, 期望 %v", c.in, got, c.want)
			}
			for i := range gs {
				if gs[i] != ws[i] {
					t.Fatalf("ensureSQLGraders(%v)=%v, 期望 %v", c.in, got, c.want)
				}
			}
			for _, g := range got {
				if g == "rouge" || g == "bleu" {
					t.Errorf("ensureSQLGraders 未剔除生成类评分器 %q: %v", g, got)
				}
			}
		})
	}
}

// TestEnsureSQLGradersDoesNotMutateInput 确认不原地改写调用方切片。
func TestEnsureSQLGradersDoesNotMutateInput(t *testing.T) {
	in := []string{"rouge", "bleu"}
	orig := append([]string(nil), in...)
	_ = ensureSQLGraders(in)
	for i := range in {
		if in[i] != orig[i] {
			t.Fatalf("ensureSQLGraders 原地改写了输入切片: %v != %v", in, orig)
		}
	}
}
