package experience

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	domain_experience "link/internal/model/agent/experience"
	domain_conversation "link/internal/model/conversation"
)

// fakeLLM 返回预置回复，忽略入参。
type fakeLLM struct {
	reply string
	err   error
	gotIn []*schema.Message
}

func (f *fakeLLM) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	f.gotIn = in
	if f.err != nil {
		return nil, f.err
	}
	return schema.AssistantMessage(f.reply, nil), nil
}

func sampleMessages() []*domain_conversation.Message {
	return []*domain_conversation.Message{
		{Role: domain_conversation.RoleUser, Content: "帮我统计上月各城市营收"},
		{Role: domain_conversation.RoleAssistant, Content: "已用 sql_execute 查询并生成图表"},
	}
}

func TestSummarize_ParsesJSONWithFence(t *testing.T) {
	reply := "```json\n" + `{
      "title":"按城市统计月度营收",
      "problem":"用户想看上月各城市营收",
      "solution":"用 sql_execute 聚合 city 维度并出图",
      "tools":["sql_execute","render_ui","sql_execute"],
      "tags":["电商分析","SQL"]
    }` + "\n```"

	s := NewSummarizer(&fakeLLM{reply: reply})
	exp, err := s.Summarize(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if exp.Title != "按城市统计月度营收" {
		t.Errorf("title = %q", exp.Title)
	}
	// tools 去重：两个 sql_execute 应折叠为一个。
	if len(exp.Tools) != 2 {
		t.Errorf("tools dedup failed: %v", exp.Tools)
	}
	if len(exp.Tags) != 2 {
		t.Errorf("tags = %v", exp.Tags)
	}
}

func TestSummarize_ParsesConfidenceAndClamps(t *testing.T) {
	reply := `{"title":"t","problem":"p","solution":"s","tools":["sql_execute"],"tags":["SQL"],"success":true,"confidence":150}`
	exp, err := NewSummarizer(&fakeLLM{reply: reply}).Summarize(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if exp.Confidence != 100 {
		t.Errorf("越界置信度应夹到 100, got %d", exp.Confidence)
	}
}

func TestSummarize_SuccessFalseCollapsesToEmpty(t *testing.T) {
	// success=false（会话未真正解决）→ 折叠为空 + 置信 0，交由 worker 空值 skipped 路径处理。
	reply := `{"title":"看着像成功","problem":"p","solution":"s","tools":[],"tags":[],"success":false,"confidence":88}`
	exp, err := NewSummarizer(&fakeLLM{reply: reply}).Summarize(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if exp.Title != "" || exp.Problem != "" || exp.Solution != "" {
		t.Errorf("success=false 应折叠为空, got title=%q problem=%q solution=%q", exp.Title, exp.Problem, exp.Solution)
	}
	if exp.Confidence != 0 {
		t.Errorf("success=false 置信度应归零, got %d", exp.Confidence)
	}
}

func TestSummarize_MissingSuccessKeepsExperience(t *testing.T) {
	// 模型未给 success 字段（指针为 nil）→ 不折叠，正常保留经验与置信度。
	reply := `{"title":"t","problem":"p","solution":"s","tools":[],"tags":[],"confidence":75}`
	exp, err := NewSummarizer(&fakeLLM{reply: reply}).Summarize(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if exp.Title != "t" || exp.Confidence != 75 {
		t.Errorf("缺 success 字段应保留经验, got title=%q conf=%d", exp.Title, exp.Confidence)
	}
}

func TestSummarize_ParsesSkillFields(t *testing.T) {
	reply := `{"title":"t","problem":"p","solution":"s","tools":["sql_execute"],"tags":["SQL"],"success":true,"confidence":90,"skill_worthy":true,"skill_instructions":"1. 先取 schema\n2. 聚合"}`
	exp, err := NewSummarizer(&fakeLLM{reply: reply}).Summarize(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if !exp.SkillWorthy {
		t.Error("skill_worthy 应解析为 true")
	}
	if !strings.Contains(exp.SkillInstructions, "聚合") {
		t.Errorf("skill_instructions 未解析: %q", exp.SkillInstructions)
	}
}

func TestSummarize_SkillWorthyWithoutInstructionsDowngrades(t *testing.T) {
	// skill_worthy=true 但没给指引正文 → 无正文不成技能，降级为普通经验。
	reply := `{"title":"t","problem":"p","solution":"s","tools":[],"tags":[],"success":true,"confidence":90,"skill_worthy":true,"skill_instructions":"   "}`
	exp, err := NewSummarizer(&fakeLLM{reply: reply}).Summarize(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if exp.SkillWorthy {
		t.Error("空指引应把 skill_worthy 降级为 false")
	}
	if exp.Title != "t" {
		t.Errorf("普通经验应保留, got title=%q", exp.Title)
	}
}

func TestSummarize_SuccessFalseClearsSkill(t *testing.T) {
	// success=false → 连同技能字段一并撤销。
	reply := `{"title":"t","problem":"p","solution":"s","tools":[],"tags":[],"success":false,"confidence":90,"skill_worthy":true,"skill_instructions":"steps"}`
	exp, err := NewSummarizer(&fakeLLM{reply: reply}).Summarize(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if exp.SkillWorthy || exp.SkillInstructions != "" {
		t.Errorf("success=false 应撤销技能, got worthy=%v instr=%q", exp.SkillWorthy, exp.SkillInstructions)
	}
}

func TestSummarize_InvalidJSONErrors(t *testing.T) {
	s := NewSummarizer(&fakeLLM{reply: "这不是 JSON"})
	if _, err := s.Summarize(context.Background(), sampleMessages()); err == nil {
		t.Fatal("expected error on non-JSON output")
	}
}

func TestSummarize_EmptyTranscriptErrors(t *testing.T) {
	s := NewSummarizer(&fakeLLM{reply: "{}"})
	msgs := []*domain_conversation.Message{{Role: "user", Content: "   "}}
	if _, err := s.Summarize(context.Background(), msgs); err == nil {
		t.Fatal("expected error on empty transcript")
	}
}

func TestBuildTranscript_TruncatesLongContent(t *testing.T) {
	s := NewSummarizer(&fakeLLM{})
	long := strings.Repeat("x", s.maxContentChars+500)
	tr := s.buildTranscript([]*domain_conversation.Message{
		{Role: "user", Content: long},
	})
	if !strings.Contains(tr, "截断") {
		t.Error("expected truncation marker")
	}
}

func TestGraphSink_NilRepoNoop(t *testing.T) {
	sink := NewGraphSink(nil)
	if err := sink.Write(context.Background(), &domain_experience.Experience{ID: 1}); err != nil {
		t.Fatalf("nil repo should be no-op, got %v", err)
	}
}

func TestExtractJSON_Balanced(t *testing.T) {
	in := "prefix {\"a\": {\"b\": 1}} suffix"
	got := extractJSON(in)
	if got != `{"a": {"b": 1}}` {
		t.Errorf("extractJSON = %q", got)
	}
}
