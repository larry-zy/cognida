package context

import (
	stdctx "context"
	"strings"
	"testing"
)

func TestWindower_NoOverflowKeepsAll(t *testing.T) {
	w := NewWindower(WindowConfig{KeepRecentTurns: 3}, nil)
	turns := []Turn{
		{Role: "user", Content: "问1"},
		{Role: "assistant", Content: "答1"},
	}
	res, err := w.Apply(stdctx.Background(), turns)
	if err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	if res.Summary != "" {
		t.Errorf("no overflow should yield empty summary, got %q", res.Summary)
	}
	if len(res.Recent) != 2 {
		t.Errorf("expected all 2 turns kept, got %d", len(res.Recent))
	}
}

func TestWindower_SummarizesOlderPreservingResultID(t *testing.T) {
	w := NewWindower(WindowConfig{KeepRecentTurns: 2}, nil)
	turns := []Turn{
		{Role: "assistant", Content: "查询完成，结果见 rs_alpha", Conclusion: "月度营收已就绪 rs_alpha"},
		{Role: "assistant", Content: "又跑了一次", ResultIDs: []string{"rs_beta"}},
		{Role: "user", Content: "最近一轮问题"},
		{Role: "assistant", Content: "最近一轮回答"},
	}
	res, err := w.Apply(stdctx.Background(), turns)
	if err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	// 保留最近 2 轮原文。
	if len(res.Recent) != 2 || res.Recent[0].Content != "最近一轮问题" {
		t.Fatalf("recent window wrong: %+v", res.Recent)
	}
	// 摘要非空且包含两个 result_id。
	if res.Summary == "" {
		t.Fatal("expected non-empty summary for overflow turns")
	}
	for _, id := range []string{"rs_alpha", "rs_beta"} {
		if !strings.Contains(res.Summary, id) {
			t.Errorf("summary must preserve %s, got: %s", id, res.Summary)
		}
	}
	// PreservedResultIDs 去重且排序。
	if len(res.PreservedResultIDs) != 2 ||
		res.PreservedResultIDs[0] != "rs_alpha" || res.PreservedResultIDs[1] != "rs_beta" {
		t.Errorf("preserved ids = %v, want [rs_alpha rs_beta]", res.PreservedResultIDs)
	}
}

func TestCollectResultIDs_DedupAndInline(t *testing.T) {
	turns := []Turn{
		{Content: "见 rs_x 和 rs_x", ResultIDs: []string{"rs_y"}},
		{Content: "无 id"},
		{Conclusion: "又见 rs_y"},
	}
	got := collectResultIDs(turns)
	if len(got) != 2 || got[0] != "rs_x" || got[1] != "rs_y" {
		t.Errorf("collectResultIDs = %v, want [rs_x rs_y]", got)
	}
	if collectResultIDs([]Turn{{Content: "空"}}) != nil {
		t.Error("no ids should return nil")
	}
}

func TestEnsureResultIDsPresent_AppendsMissing(t *testing.T) {
	out := ensureResultIDsPresent("摘要含 rs_a", []string{"rs_a", "rs_b"})
	if !strings.Contains(out, "rs_a") || !strings.Contains(out, "rs_b") {
		t.Errorf("missing id not appended: %s", out)
	}
	// 已全含时不改动。
	if got := ensureResultIDsPresent("有 rs_a rs_b", []string{"rs_a", "rs_b"}); got != "有 rs_a rs_b" {
		t.Errorf("should be unchanged, got %s", got)
	}
	// 空摘要时用尾巴替代。
	if got := ensureResultIDsPresent("", []string{"rs_a"}); !strings.Contains(got, "rs_a") {
		t.Errorf("empty summary should still carry id, got %s", got)
	}
}

func TestExtractiveSummarizer_UsesConclusionOrCollapsedContent(t *testing.T) {
	older := []Turn{
		{Role: "assistant", Conclusion: "结论A"},
		{Role: "user", Content: "很长的内容    带  多余   空白"},
	}
	s, err := ExtractiveSummarizer{}.Summarize(stdctx.Background(), older, []string{"rs_z"})
	if err != nil {
		t.Fatalf("Summarize error = %v", err)
	}
	if !strings.Contains(s, "结论A") {
		t.Error("summary should use conclusion when present")
	}
	if !strings.Contains(s, "很长的内容 带 多余 空白") {
		t.Errorf("content should be whitespace-collapsed, got: %s", s)
	}
	if !strings.Contains(s, "rs_z") {
		t.Error("summary should list preserved result id")
	}
}
