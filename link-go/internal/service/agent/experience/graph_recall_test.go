package experience

import (
	"context"
	"errors"
	"testing"

	"link/internal/model/knowledge"
)

// fakeGraphRepo 只实现 Recall 用到的 GetGraph，其余方法未被调用。
// 通过嵌入接口获得其余方法的默认（nil）实现——调用它们会 panic，但本测试不会触及。
type fakeGraphRepo struct {
	knowledge.GraphRepository
	data *knowledge.GraphData
	err  error
	got  knowledge.NameSpace
}

func (f *fakeGraphRepo) GetGraph(ctx context.Context, ns knowledge.NameSpace) (*knowledge.GraphData, error) {
	f.got = ns
	return f.data, f.err
}

// expSubgraph 构造一幅经验子图：两条经验，各自 RELATED_TO 到若干话题/工具节点。
func expSubgraph() *knowledge.GraphData {
	rel := func(id, exp, other string) *knowledge.GraphRelation {
		return &knowledge.GraphRelation{ID: id, Source: exp, Target: other, Type: string(knowledge.RelationTypeRelatedTo)}
	}
	return &knowledge.GraphData{
		Node: []*knowledge.GraphNode{
			{ID: "exp_1", Name: "经验：按城市统计月度营收", EntityType: "experience",
				Properties: map[string]string{"problem": "看各城市营收", "solution": "sql 聚合 city 出图"}},
			{ID: "exp_2", Name: "经验：季节性大促选品", EntityType: "experience",
				Properties: map[string]string{"problem": "大促选品", "solution": "按季节性分布筛选"}},
			{ID: "topic_电商分析", Name: "电商分析", EntityType: "topic"},
			{ID: "topic_季节性大促", Name: "季节性大促", EntityType: "topic"},
			{ID: "tool_sql_execute", Name: "sql_execute", EntityType: "tool"},
		},
		Relation: []*knowledge.GraphRelation{
			rel("r1", "经验：按城市统计月度营收", "电商分析"),
			rel("r2", "经验：按城市统计月度营收", "sql_execute"),
			rel("r3", "经验：季节性大促选品", "季节性大促"),
			rel("r4", "经验：季节性大促选品", "电商分析"),
		},
	}
}

// confSubgraph 构造两条「原始匹配得分相同、置信度不同」的经验，均 RELATED_TO「电商分析」。
// prop 传入各自的 confidence 属性（空串=不写该属性，模拟置信度上线前的历史节点）。
func confSubgraph(confHigh, confLow string) *knowledge.GraphData {
	propHigh := map[string]string{"problem": "高置信", "solution": "高置信解法"}
	propLow := map[string]string{"problem": "低置信", "solution": "低置信解法"}
	if confHigh != "" {
		propHigh["confidence"] = confHigh
	}
	if confLow != "" {
		propLow["confidence"] = confLow
	}
	return &knowledge.GraphData{
		Node: []*knowledge.GraphNode{
			{ID: "exp_10", Name: "经验：高置信经验", EntityType: "experience", Properties: propHigh},
			{ID: "exp_11", Name: "经验：低置信经验", EntityType: "experience", Properties: propLow},
			{ID: "topic_电商分析", Name: "电商分析", EntityType: "topic"},
		},
		Relation: []*knowledge.GraphRelation{
			{ID: "cr1", Source: "经验：高置信经验", Target: "电商分析", Type: string(knowledge.RelationTypeRelatedTo)},
			{ID: "cr2", Source: "经验：低置信经验", Target: "电商分析", Type: string(knowledge.RelationTypeRelatedTo)},
		},
	}
}

func TestRecall_ConfidenceWeightsRanking(t *testing.T) {
	// 原始匹配得分相同（都命中「电商分析」=4），置信度高者应排前。
	r := NewGraphRecaller(&fakeGraphRepo{data: confSubgraph("90", "20")})
	got := r.Recall(context.Background(), 1, "电商分析", 3)
	if len(got) != 2 {
		t.Fatalf("应召回 2 条, got %d: %+v", len(got), got)
	}
	if got[0].Title != "高置信经验" {
		t.Errorf("同分下高置信应排首位, got %q (conf=%d)", got[0].Title, got[0].Confidence)
	}
	if got[0].Score != got[1].Score {
		t.Errorf("两条原始匹配得分应相同: %d vs %d", got[0].Score, got[1].Score)
	}
	if got[0].Confidence != 90 || got[1].Confidence != 20 {
		t.Errorf("置信度应从 Properties 解析: %d / %d", got[0].Confidence, got[1].Confidence)
	}
}

func TestRecall_MissingConfidenceTreatedAsNeutral(t *testing.T) {
	// 高置信经验缺 confidence 属性（历史节点）→ 按 100 中性处理，
	// 应压过带 confidence=20 的另一条，而非被误判为 0 沉底。
	r := NewGraphRecaller(&fakeGraphRepo{data: confSubgraph("", "20")})
	got := r.Recall(context.Background(), 1, "电商分析", 3)
	if len(got) != 2 {
		t.Fatalf("应召回 2 条, got %d", len(got))
	}
	if got[0].Title != "高置信经验" || got[0].Confidence != 100 {
		t.Errorf("缺 confidence 应按 100 中性处理并排前, got %q conf=%d", got[0].Title, got[0].Confidence)
	}
}

func TestRecall_NilRepoAndEmptyQuery(t *testing.T) {
	if got := (*GraphRecaller)(nil).Recall(context.Background(), 1, "x", 3); got != nil {
		t.Errorf("nil recaller 应返回 nil, got %v", got)
	}
	r := NewGraphRecaller(&fakeGraphRepo{data: expSubgraph()})
	if got := r.Recall(context.Background(), 1, "   ", 3); got != nil {
		t.Errorf("空 query 应返回 nil, got %v", got)
	}
}

func TestRecall_GraphErrorDegradesToEmpty(t *testing.T) {
	r := NewGraphRecaller(&fakeGraphRepo{err: errors.New("neo4j down")})
	if got := r.Recall(context.Background(), 1, "电商分析", 3); got != nil {
		t.Errorf("图谱报错应降级为 nil, got %v", got)
	}
}

func TestRecall_UsesTenantGlobalNamespace(t *testing.T) {
	repo := &fakeGraphRepo{data: expSubgraph()}
	NewGraphRecaller(repo).Recall(context.Background(), 42, "电商分析", 3)
	if repo.got.TenantID != "42" || repo.got.KnowledgeBaseID != "" {
		t.Errorf("命名空间应为租户全域(kb=\"\"), got %+v", repo.got)
	}
}

func TestRecall_MatchesByTopicAndRanks(t *testing.T) {
	r := NewGraphRecaller(&fakeGraphRepo{data: expSubgraph()})

	// 问题里出现「季节性大促」→ exp_2 命中「季节性大促」(5 runes) + 无「电商分析」，
	// exp_1 只经 RELATED_TO 关联「电商分析/sql_execute」，问题里都没出现 → 不召回。
	got := r.Recall(context.Background(), 1, "帮我做季节性大促选品", 3)
	if len(got) != 1 {
		t.Fatalf("应只召回 1 条(季节性大促), got %d: %+v", len(got), got)
	}
	if got[0].Title != "季节性大促选品" {
		t.Errorf("召回标题应去掉「经验：」前缀, got %q", got[0].Title)
	}
	if got[0].Solution != "按季节性分布筛选" {
		t.Errorf("solution 应从 Properties 取出, got %q", got[0].Solution)
	}
}

func TestRecall_MultiSignalOutranksAndLimit(t *testing.T) {
	r := NewGraphRecaller(&fakeGraphRepo{data: expSubgraph()})

	// 问题同时含「电商分析」「季节性大促」：
	//   exp_1: 命中「电商分析」(4) = 4
	//   exp_2: 命中「季节性大促」(5)+「电商分析」(4) = 9 → 排前
	got := r.Recall(context.Background(), 1, "电商分析里的季节性大促怎么做", 3)
	if len(got) != 2 {
		t.Fatalf("应召回 2 条, got %d: %+v", len(got), got)
	}
	if got[0].Title != "季节性大促选品" {
		t.Errorf("多信号命中应排首位, got %q (score=%d)", got[0].Title, got[0].Score)
	}
	if got[0].Score <= got[1].Score {
		t.Errorf("首位得分应更高: %d vs %d", got[0].Score, got[1].Score)
	}

	// limit=1 截断。
	if one := r.Recall(context.Background(), 1, "电商分析里的季节性大促怎么做", 1); len(one) != 1 {
		t.Errorf("limit=1 应截断为 1 条, got %d", len(one))
	}
}
