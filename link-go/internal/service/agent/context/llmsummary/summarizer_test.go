package llmsummary

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	ctxeng "link/internal/service/agent/context"
)

// fakeLLM 是可编排的最小生成句柄：按需返回固定摘要、错误或空产出，并记录收到的输入。
type fakeLLM struct {
	reply   string
	err     error
	calls   int
	lastIn  []*schema.Message
	returns *schema.Message // 非 nil 时优先于 reply（用于构造空 Content 的响应）
}

func (f *fakeLLM) Generate(_ context.Context, in []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.calls++
	f.lastIn = in
	if f.err != nil {
		return nil, f.err
	}
	if f.returns != nil {
		return f.returns, nil
	}
	return schema.AssistantMessage(f.reply, nil), nil
}

func older() []ctxeng.Turn {
	return []ctxeng.Turn{
		{Role: "user", Content: "华东 Q2 各城市销售额多少"},
		{Role: "assistant", Content: "已查出各城市销售额，明细见 rs-east-q2"},
	}
}

// LLM 正常返回时应直接采用其摘要正文（去空白），并真正调用了一次模型。
func TestSummarize_UsesLLMReply(t *testing.T) {
	f := &fakeLLM{reply: "  华东Q2销售额已汇总（rs-east-q2）  "}
	s := New(f)

	got, err := s.Summarize(context.Background(), older(), []string{"rs-east-q2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.calls != 1 {
		t.Fatalf("expected 1 LLM call, got %d", f.calls)
	}
	if got != "华东Q2销售额已汇总（rs-east-q2）" {
		t.Fatalf("expected trimmed LLM reply, got %q", got)
	}
}

// preserved 里的 result_id 必须出现在喂给 LLM 的 user prompt 里（强约束原样保留）。
func TestSummarize_PromptCarriesPreservedIDs(t *testing.T) {
	f := &fakeLLM{reply: "ok"}
	s := New(f)

	if _, err := s.Summarize(context.Background(), older(), []string{"rs-east-q2"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.lastIn) == 0 {
		t.Fatal("expected messages passed to LLM")
	}
	userPrompt := f.lastIn[len(f.lastIn)-1].Content
	if !strings.Contains(userPrompt, "rs-east-q2") {
		t.Fatalf("expected preserved result_id in prompt, got %q", userPrompt)
	}
}

// LLM 报错时降级到确定性抽取式摘要，且绝不把错误抛给主循环。
func TestSummarize_FallsBackOnError(t *testing.T) {
	f := &fakeLLM{err: errors.New("boom")}
	s := New(f)

	got, err := s.Summarize(context.Background(), older(), []string{"rs-east-q2"})
	if err != nil {
		t.Fatalf("fallback must not surface error, got %v", err)
	}
	want, _ := ctxeng.ExtractiveSummarizer{}.Summarize(context.Background(), older(), []string{"rs-east-q2"})
	if got != want {
		t.Fatalf("expected extractive fallback %q, got %q", want, got)
	}
}

// LLM 返回空 Content 时同样降级（等价于不可用）。
func TestSummarize_FallsBackOnEmptyReply(t *testing.T) {
	f := &fakeLLM{returns: schema.AssistantMessage("   ", nil)}
	s := New(f)

	got, err := s.Summarize(context.Background(), older(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, _ := ctxeng.ExtractiveSummarizer{}.Summarize(context.Background(), older(), nil)
	if got != want {
		t.Fatalf("expected extractive fallback on empty reply, got %q", got)
	}
	if f.calls != 1 {
		t.Fatalf("expected LLM to be attempted once, got %d calls", f.calls)
	}
}

// llm 为 nil 时退化为纯抽取式，零回归，且不 panic。
func TestSummarize_NilLLMDegradesToExtractive(t *testing.T) {
	s := New(nil)

	got, err := s.Summarize(context.Background(), older(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, _ := ctxeng.ExtractiveSummarizer{}.Summarize(context.Background(), older(), nil)
	if got != want {
		t.Fatalf("expected extractive result with nil llm, got %q", got)
	}
}

// 空历史直接走降级，不调用 LLM（无内容可摘要）。
func TestSummarize_EmptyOlderSkipsLLM(t *testing.T) {
	f := &fakeLLM{reply: "should-not-be-used"}
	s := New(f)

	got, err := s.Summarize(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.calls != 0 {
		t.Fatalf("expected no LLM call for empty older, got %d", f.calls)
	}
	want, _ := ctxeng.ExtractiveSummarizer{}.Summarize(context.Background(), nil, nil)
	if got != want {
		t.Fatalf("expected extractive fallback for empty older, got %q", got)
	}
}

// buildTranscript 超过上限时从尾部保留（越靠近当前对话越重要），并加省略标记。
func TestBuildTranscript_TailPreservingCap(t *testing.T) {
	s := New(nil)
	s.maxInputChars = 20

	turns := []ctxeng.Turn{
		{Role: "user", Content: strings.Repeat("A", 100)},
		{Role: "assistant", Content: "结论尾部XYZ"},
	}
	tr := s.buildTranscript(turns)
	if !strings.Contains(tr, "结论尾部XYZ") {
		t.Fatalf("expected tail content preserved, got %q", tr)
	}
	if !strings.Contains(tr, "略") {
		t.Fatalf("expected elision marker when over cap, got %q", tr)
	}
	if strings.Count(tr, "A") > 20 {
		t.Fatalf("expected head truncated under cap, got %q", tr)
	}
}
