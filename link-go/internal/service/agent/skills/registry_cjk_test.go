package skills

import "testing"

// 构造与真实 SKILL.md 同风格的中文文案 Skill，用于验证 CJK 匹配确实生效。
func newCJKTestRegistry(t *testing.T) SkillRegistry {
	t.Helper()
	reg := NewSkillRegistry()
	seed := []*Skill{
		{
			Name:         "text2sql-adhoc",
			Description:  "即席取数技能，把一句话的具体数据问题翻译成 SQL 查询数据库并执行返回结果。",
			WhenToUse:    "当用户要查询一个具体的数字、明细或清单（如上个月有多少订单、按金额分析客户），且不涉及治理指标口径、也不需要整合成报告时使用。",
			Category:     "data",
			Tags:         []string{"text2sql", "sql", "query"},
			AllowedTools: []string{"sql_execute", "get_schema"},
		},
		{
			Name:        "doc-qa",
			Description: "文档问答技能，从知识库中语义检索相关文档片段并据此作答。",
			WhenToUse:   "当用户的问题需要从知识库或文档中查找答案，依赖非结构化文本内容而非数据库取数时使用。",
			Category:    "retrieval",
			Tags:        []string{"rag", "retrieval"},
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
		{"取数分析", "帮我查询数据库里上个月的订单并分析一下", "text2sql-adhoc"},
		{"知识库检索", "从知识库里检索一下相关的文档知识", "doc-qa"},
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

// 名称校验须接受中日韩等 Unicode 字母名——否则经验沉淀出的中文名技能注册即被拒。
// 该用例与 experience.SkillSink 的 slugify（保留 \p{Han}）是配套契约。
func TestIsValidSkillName_UnicodeLetters(t *testing.T) {
	valid := []string{
		"text2sql-adhoc",
		"电商核心经营指标综合报告生成",
		"统计用户总数",
		"report_2026",
		"data analysis",
	}
	for _, n := range valid {
		if !IsValidSkillName(n) {
			t.Errorf("IsValidSkillName(%q) = false, want true", n)
		}
	}
	invalid := []string{
		"",
		"bad/name",
		"has.dot",
		"emoji😀name",
		"slash\\name",
	}
	for _, n := range invalid {
		if IsValidSkillName(n) {
			t.Errorf("IsValidSkillName(%q) = true, want false", n)
		}
	}

	// 契约验证：中文标题落盘后能真正注册进 registry（复现并锁死原缺陷）。
	reg := NewSkillRegistry()
	cjk := &Skill{Name: "电商核心经营指标综合报告生成", Description: "d", WhenToUse: "w", Category: "experience"}
	if err := reg.Register(cjk); err != nil {
		t.Fatalf("register CJK-named skill: %v", err)
	}
	if _, ok := reg.Get("电商核心经营指标综合报告生成"); !ok {
		t.Error("CJK-named skill not retrievable after register")
	}
}
