package experience

import (
	"context"
	"os"
	"path/filepath"
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
      "tags":["电商分析","SQL"],
      "skill_worthy":true,
      "skill_instructions":"1. 明确时间范围\n2. 按 city 分组 SUM(revenue)\n3. render_ui 出柱状图"
    }` + "\n```"

	s := NewSummarizer(&fakeLLM{reply: reply})
	exp, err := s.Summarize(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if exp.Title != "按城市统计月度营收" {
		t.Errorf("title = %q", exp.Title)
	}
	if !exp.SkillWorthy {
		t.Error("expected skill_worthy=true")
	}
	// tools 去重：两个 sql_execute 应折叠为一个。
	if len(exp.Tools) != 2 {
		t.Errorf("tools dedup failed: %v", exp.Tools)
	}
	if len(exp.Tags) != 2 {
		t.Errorf("tags = %v", exp.Tags)
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

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World!!!":  "hello-world",
		"  多个   空格 test ": "多个-空格-test",
		"---":             "",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSkillSink_WritesAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	sink := NewSkillSink(root)
	exp := &domain_experience.Experience{
		ID:                7,
		SessionID:         "sess-1",
		Title:             "按城市统计营收",
		Problem:           "用户想看各城市营收",
		Solution:          "sql 聚合",
		Tools:             []string{"sql_execute"},
		Tags:              []string{"SQL"},
		SkillWorthy:       true,
		SkillInstructions: "按 city 分组 SUM(revenue)",
	}

	name, err := sink.Write(context.Background(), exp)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if name == "" {
		t.Fatal("expected non-empty skill name")
	}

	path := filepath.Join(root, name, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("SKILL.md not written: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "name: ") || !strings.Contains(body, "按 city 分组") {
		t.Errorf("SKILL.md missing expected content:\n%s", body)
	}
	if !strings.Contains(body, "disable_model_invocation: true") {
		t.Error("expected disable_model_invocation frontmatter")
	}

	// 幂等：再次写入不报错、不覆盖，返回同名。
	name2, err := sink.Write(context.Background(), exp)
	if err != nil || name2 != name {
		t.Fatalf("idempotent write failed: name2=%q err=%v", name2, err)
	}
}

func TestSkillSink_SkipsWhenNotWorthy(t *testing.T) {
	sink := NewSkillSink(t.TempDir())
	exp := &domain_experience.Experience{ID: 1, SkillWorthy: false, SkillInstructions: "x"}
	name, err := sink.Write(context.Background(), exp)
	if err != nil || name != "" {
		t.Fatalf("expected skip, got name=%q err=%v", name, err)
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
