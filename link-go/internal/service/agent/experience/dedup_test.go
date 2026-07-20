package experience

import (
	"testing"

	domain_experience "link/internal/model/agent/experience"
)

func exp(session string, tools, tags []string) *domain_experience.Experience {
	return &domain_experience.Experience{SessionID: session, Tools: tools, Tags: tags}
}

func TestJaccard(t *testing.T) {
	a := featureSet(exp("a", []string{"sql_execute", "render_ui"}, []string{"电商"}))
	b := featureSet(exp("b", []string{"sql_execute", "render_ui"}, []string{"电商"}))
	if got := jaccard(a, b); got != 1.0 {
		t.Fatalf("完全相同应为 1.0，得 %v", got)
	}
	// {sql,render,电商} vs {sql,render,报表}: 交 2 / 并 4 = 0.5
	c := featureSet(exp("c", []string{"sql_execute", "render_ui"}, []string{"报表"}))
	if got := jaccard(a, c); got != 0.5 {
		t.Fatalf("期望 0.5，得 %v", got)
	}
	if got := jaccard(a, map[string]struct{}{}); got != 0 {
		t.Fatalf("空集应为 0，得 %v", got)
	}
}

func TestFindDuplicate_AboveThreshold(t *testing.T) {
	d := NewDeduplicator(0.8)
	target := exp("new", []string{"sql_execute", "render_ui"}, []string{"电商"})
	cands := []*domain_experience.Experience{
		{SessionID: "old-1", SkillName: "ecommerce-report", Tools: []string{"sql_execute", "render_ui"}, Tags: []string{"电商"}},
	}
	dup := d.FindDuplicate(target, cands)
	if dup == nil || dup.SkillName != "ecommerce-report" {
		t.Fatalf("应命中近重复并复用 ecommerce-report，得 %+v", dup)
	}
}

func TestFindDuplicate_BelowThreshold(t *testing.T) {
	d := NewDeduplicator(0.8)
	target := exp("new", []string{"sql_execute"}, []string{"电商"})
	// 交 1 / 并 3 ≈ 0.33 < 0.8
	cands := []*domain_experience.Experience{
		{SessionID: "old-1", SkillName: "x", Tools: []string{"http_call"}, Tags: []string{"电商", "报表"}},
	}
	if dup := d.FindDuplicate(target, cands); dup != nil {
		t.Fatalf("相似度不足不应合并，得 %+v", dup)
	}
}

func TestFindDuplicate_PicksHighest(t *testing.T) {
	d := NewDeduplicator(0.8)
	target := exp("new", []string{"a", "b", "c"}, nil)
	cands := []*domain_experience.Experience{
		{SessionID: "o1", SkillName: "low", Tools: []string{"a", "b"}, Tags: []string{"x"}},  // 交2/并4=0.5
		{SessionID: "o2", SkillName: "high", Tools: []string{"a", "b", "c", "d"}, Tags: nil}, // 交3/并4=0.75
		{SessionID: "o3", SkillName: "best", Tools: []string{"a", "b", "c"}, Tags: nil},      // 交3/并3=1.0
	}
	dup := d.FindDuplicate(target, cands)
	if dup == nil || dup.SkillName != "best" {
		t.Fatalf("应选相似度最高者 best，得 %+v", dup)
	}
}

func TestFindDuplicate_Disabled(t *testing.T) {
	d := NewDeduplicator(0)
	if d.Enabled() {
		t.Fatal("阈值 0 应关闭去重")
	}
	target := exp("new", []string{"a"}, []string{"b"})
	cands := []*domain_experience.Experience{
		{SessionID: "old", SkillName: "x", Tools: []string{"a"}, Tags: []string{"b"}},
	}
	if dup := d.FindDuplicate(target, cands); dup != nil {
		t.Fatalf("关闭时不应合并，得 %+v", dup)
	}
}

func TestFindDuplicate_IgnoresSameSessionAndEmpty(t *testing.T) {
	d := NewDeduplicator(0.8)
	target := exp("same", []string{"a", "b"}, nil)

	// 同会话（自身）应跳过
	same := []*domain_experience.Experience{
		{SessionID: "same", SkillName: "self", Tools: []string{"a", "b"}, Tags: nil},
	}
	if dup := d.FindDuplicate(target, same); dup != nil {
		t.Fatalf("同会话应跳过，得 %+v", dup)
	}

	// 目标无 tools/tags 特征时不合并
	empty := exp("new", nil, nil)
	cands := []*domain_experience.Experience{
		{SessionID: "old", SkillName: "x", Tools: []string{"a"}, Tags: []string{"b"}},
	}
	if dup := d.FindDuplicate(empty, cands); dup != nil {
		t.Fatalf("目标无特征应不合并，得 %+v", dup)
	}

	// 大小写/空白归一化后应命中
	target2 := exp("t2", []string{" SQL_Execute ", "Render_UI"}, nil)
	cands2 := []*domain_experience.Experience{
		{SessionID: "o", SkillName: "norm", Tools: []string{"sql_execute", "render_ui"}, Tags: nil},
	}
	if dup := d.FindDuplicate(target2, cands2); dup == nil || dup.SkillName != "norm" {
		t.Fatalf("归一化后应命中 norm，得 %+v", dup)
	}
}
