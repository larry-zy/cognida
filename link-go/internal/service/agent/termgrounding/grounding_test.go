package termgrounding

import (
	"context"
	"testing"

	"link/internal/model/semantic"
)

// bundle 构造一个含同义词的语义模型：
//   - 指标 营收（同义词 GMV/成交额），营业额
//   - 维度 区域（同义词 大区/地区）
//   - 度量 订单数
// 其中 "地区" 同时是 区域(dim) 的同义词，也当作 营业额 无关——用于歧义构造见测试。
func bundle() *semantic.ModelBundle {
	return &semantic.ModelBundle{
		Model: &semantic.SemanticModel{Name: "sales", Version: 2},
		Metrics: []*semantic.Metric{
			{Name: "营收", Synonyms: []string{"GMV", "成交额"}},
			{Name: "营业额", Synonyms: []string{"成交额"}}, // 与 营收 共享 "成交额" → 歧义
		},
		Measures: []*semantic.Measure{
			{Name: "订单数"},
		},
		Dimensions: []*semantic.Dimension{
			{Name: "区域", Synonyms: []string{"大区", "地区"}},
		},
	}
}

func TestGround_NameExactMatch(t *testing.T) {
	g := NewGrounder(nil)
	res := g.Ground(context.Background(), 1, bundle(), []string{"营收"})
	if len(res) != 1 || res[0].Resolved != "营收" || res[0].Kind != KindMetric {
		t.Fatalf("unexpected: %+v", res)
	}
	if res[0].Ambiguous || res[0].Unresolved {
		t.Errorf("should be a clean resolve: %+v", res[0])
	}
	if len(res[0].Candidates) != 1 || res[0].Candidates[0].Source != SourceName {
		t.Errorf("expected single name-source candidate: %+v", res[0].Candidates)
	}
}

func TestGround_SynonymAndCaseInsensitive(t *testing.T) {
	g := NewGrounder(nil)
	res := g.Ground(context.Background(), 1, bundle(), []string{"  gmv "})
	if res[0].Resolved != "营收" || res[0].Kind != KindMetric {
		t.Fatalf("synonym gmv should ground to 营收: %+v", res[0])
	}
	if res[0].Candidates[0].Source != SourceSynonym {
		t.Errorf("expected synonym source: %+v", res[0].Candidates)
	}
}

func TestGround_DimensionSynonym(t *testing.T) {
	g := NewGrounder(nil)
	res := g.Ground(context.Background(), 1, bundle(), []string{"大区"})
	if res[0].Resolved != "区域" || res[0].Kind != KindDimension {
		t.Fatalf("大区 should ground to 区域: %+v", res[0])
	}
}

func TestGround_AmbiguousSharedSynonym(t *testing.T) {
	g := NewGrounder(nil)
	res := g.Ground(context.Background(), 1, bundle(), []string{"成交额"})
	if !res[0].Ambiguous {
		t.Fatalf("成交额 maps to 营收 and 营业额 → must be ambiguous: %+v", res[0])
	}
	if res[0].Resolved != "" {
		t.Errorf("ambiguous term must not hard-resolve: %q", res[0].Resolved)
	}
	if len(res[0].Candidates) != 2 {
		t.Errorf("expected 2 candidates: %+v", res[0].Candidates)
	}
}

func TestGround_Unresolved(t *testing.T) {
	g := NewGrounder(nil)
	res := g.Ground(context.Background(), 1, bundle(), []string{"客单价"})
	if !res[0].Unresolved || res[0].Resolved != "" {
		t.Fatalf("客单价 not in model → unresolved: %+v", res[0])
	}
}

// stubGraph 是 GraphPort 打桩：把 "网站成交" 解析为图谱别名 "GMV"。
type stubGraph struct {
	aliases map[string][]string
	calls   int
}

func (s *stubGraph) ResolveAliases(_ context.Context, _ int64, term string) ([]string, error) {
	s.calls++
	return s.aliases[norm(term)], nil
}

func TestGround_GraphFallbackResolves(t *testing.T) {
	sg := &stubGraph{aliases: map[string][]string{"网站成交": {"GMV"}}}
	g := NewGrounder(sg)
	res := g.Ground(context.Background(), 1, bundle(), []string{"网站成交"})
	if res[0].Resolved != "营收" || res[0].Kind != KindMetric {
		t.Fatalf("graph alias GMV should ground to 营收: %+v", res[0])
	}
	if res[0].Candidates[0].Source != SourceGraph || res[0].Candidates[0].Via != "GMV" {
		t.Errorf("expected graph-source candidate via GMV: %+v", res[0].Candidates)
	}
}

func TestGround_InModelHitSkipsGraph(t *testing.T) {
	sg := &stubGraph{aliases: map[string][]string{"gmv": {"营业额"}}}
	g := NewGrounder(sg)
	// "GMV" 已是模型内同义词 → 直接命中 营收，不应触发图谱调用。
	res := g.Ground(context.Background(), 1, bundle(), []string{"GMV"})
	if res[0].Resolved != "营收" {
		t.Fatalf("in-model synonym should win: %+v", res[0])
	}
	if sg.calls != 0 {
		t.Errorf("graph must not be consulted when in-model hits, calls=%d", sg.calls)
	}
}

func TestGround_GraphAmbiguous(t *testing.T) {
	// 图谱把术语解析到两个不同概念 → 歧义反问。
	sg := &stubGraph{aliases: map[string][]string{"额度": {"营收", "营业额"}}}
	g := NewGrounder(sg)
	res := g.Ground(context.Background(), 1, bundle(), []string{"额度"})
	if !res[0].Ambiguous || res[0].Resolved != "" {
		t.Fatalf("graph-resolved multi-concept must be ambiguous: %+v", res[0])
	}
}
