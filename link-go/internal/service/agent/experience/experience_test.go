package experience

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	domain_experience "link/internal/model/agent/experience"
	domain_conversation "link/internal/model/conversation"
	"link/internal/service/agent/skills"
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

// 自进化沉淀出的中文名技能，其 name 必须能通过注册器校验，否则加载即被拒。
// 这是「沉淀 → 加载」闭环的契约测试，锁死中文名注册失败的回归。
func TestSkillSink_ChineseTitleNameIsRegistrable(t *testing.T) {
	root := t.TempDir()
	sink := NewSkillSink(root)
	exp := &domain_experience.Experience{
		ID:                42,
		SessionID:         "sess-cjk",
		Title:             "电商核心经营指标综合报告生成",
		Problem:           "用户需要一份电商核心经营指标的综合报告",
		Tools:             []string{"data_analysis"},
		SkillWorthy:       true,
		SkillInstructions: "按 GMV/订单数/客单价聚合并整合报告",
	}
	name, err := sink.Write(context.Background(), exp)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if !skills.IsValidSkillName(name) {
		t.Fatalf("generated name %q fails IsValidSkillName — 沉淀出的技能将无法注册", name)
	}
	// 真正回读并加载，确认落盘的 SKILL.md 能被注册器接收。
	res, err := NewSkillManagerLoad(t, filepath.Join(root, name))
	if err != nil {
		t.Fatalf("load generated skill: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("generated CJK-named skill %q was not registered on load", name)
	}
}

// NewSkillManagerLoad 加载单个技能目录，返回成功注册的技能名列表（测试辅助）。
func NewSkillManagerLoad(t *testing.T, skillDir string) ([]string, error) {
	t.Helper()
	m := skills.NewSkillManager()
	// 传入父目录，LoadSkills 会扫描其下含 SKILL.md 的子目录。
	res, err := m.LoadSkills(filepath.Dir(skillDir))
	if err != nil {
		return nil, err
	}
	if len(res.Errors) > 0 {
		return nil, fmt.Errorf("load errors: %v", res.Errors)
	}
	names := make([]string, 0, len(res.Skills))
	for _, s := range res.Skills {
		names = append(names, s.Name)
	}
	return names, nil
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
