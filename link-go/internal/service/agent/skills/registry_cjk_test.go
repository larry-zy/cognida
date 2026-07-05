package skills

import "testing"

// 构造与真实 SKILL.md 同风格的中文文案 Skill，用于验证 CJK 匹配确实生效。
func newCJKTestRegistry(t *testing.T) SkillRegistry {
	t.Helper()
	reg := NewSkillRegistry()
	seed := []*Skill{
		{
			Name:         "data-analysis",
			Description:  "专注于数据查询和分析的技能，通过 SQL 查询获取和分析数据。",
			WhenToUse:    "当任务需要查询数据库、分析数据、生成报表或执行数据统计时使用此技能。",
			Category:     "data",
			Tags:         []string{"sql", "database", "analytics"},
			AllowedTools: []string{"sql_execute", "get_schema"},
		},
		{
			Name:        "rag-search",
			Description: "专注于知识库检索的技能，通过向量检索和混合检索获取相关知识。",
			WhenToUse:   "当任务需要从知识库中检索相关信息、查找文档或获取上下文知识时使用此技能。",
			Category:    "retrieval",
			Tags:        []string{"rag", "retrieval"},
		},
		{
			Name:        "code-review",
			Description: "专注于代码审查和分析的技能，帮助发现代码问题、改进代码质量。",
			WhenToUse:   "当任务涉及代码审查、代码质量分析、查找代码问题时使用此技能。",
			Category:    "development",
			Tags:        []string{"code-review", "quality"},
		},
	}
	for _, s := range seed {
		if err := reg.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name, err)
		}
	}
	return reg
}

// 修复前：中文查询经 strings.Fields 坍缩为单一 term，相关度恒 ≈ 0，命中不了任何 Skill。
// 修复后：CJK bigram 重叠使中文查询能真正命中对应中文 Skill，且相关度过 0.5 阈值。
func TestMatchForTask_CJKQueryHits(t *testing.T) {
	reg := newCJKTestRegistry(t)
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"取数分析", "帮我查询数据库里上个月的订单并分析一下", "data-analysis"},
		{"知识库检索", "从知识库里检索一下相关的文档知识", "rag-search"},
		{"代码审查", "帮我做一次代码审查，看看有没有代码问题", "code-review"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			matches := reg.MatchForTask(c.query, 1)
			if len(matches) == 0 {
				t.Fatalf("query %q matched no skill (CJK matching inert)", c.query)
			}
			top := matches[0]
			if top.Skill.Name != c.want {
				t.Errorf("query %q top skill = %s, want %s (relevance=%.3f)", c.query, top.Skill.Name, c.want, top.Relevance)
			}
			if top.Relevance < 0.5 {
				t.Errorf("query %q relevance = %.3f, want ≥ 0.5 so the gate/injection engages", c.query, top.Relevance)
			}
		})
	}
}

// 无关中文查询不应误命中任一 Skill（保证阈值仍有区分度）。
func TestMatchForTask_UnrelatedCJKQueryMisses(t *testing.T) {
	reg := newCJKTestRegistry(t)
	matches := reg.MatchForTask("今天天气怎么样适合出去玩吗", 1)
	if len(matches) > 0 && matches[0].Relevance >= 0.5 {
		t.Errorf("unrelated query should not cross threshold, got %s @ %.3f", matches[0].Skill.Name, matches[0].Relevance)
	}
}

func TestCJKBigrams(t *testing.T) {
	got := cjkBigrams("数据分析")
	want := []string{"数据", "据分", "分析"}
	if len(got) != len(want) {
		t.Fatalf("cjkBigrams len = %d (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("bigram[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// 去重 + 非 CJK 断开：英文与标点不产生 bigram。
	if bs := cjkBigrams("sql query 数据 数据"); len(bs) != 1 || bs[0] != "数据" {
		t.Errorf("mixed/dup input = %v, want [数据]", bs)
	}
	if bs := cjkBigrams("hello world"); len(bs) != 0 {
		t.Errorf("pure ascii should yield no bigrams, got %v", bs)
	}
}
