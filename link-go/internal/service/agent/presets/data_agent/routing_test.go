package dataagent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// stubModel 是 BaseChatModel 的测试桩：Generate 返回预置内容或错误。
type stubModel struct {
	content string
	err     error
}

func (m *stubModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if m.err != nil {
		return nil, m.err
	}
	return schema.AssistantMessage(m.content, nil), nil
}

func (m *stubModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("not implemented")
}

func TestClassifyIntent(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    Intent
	}{
		{"取数", "查一下上个月的订单总数", IntentFetch},
		{"趋势", "最近半年的销售额趋势如何", IntentTrend},
		{"归因", "为什么这个月营收下降了", IntentAttribution},
		{"报告", "给我出一份本季度经营看板报告", IntentReport},
		{"通用有内容", "帮我看看华东区 2024 的客单价", IntentGeneral},
		{"问候歧义", "你好", IntentAmbiguous},
		{"空串歧义", "   ", IntentAmbiguous},
		{"过短歧义", "数据", IntentAmbiguous},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyIntent(c.message)
			if got.Intent != c.want {
				t.Errorf("ClassifyIntent(%q).Intent = %v, want %v", c.message, got.Intent, c.want)
			}
			if got.Playbook == "" {
				t.Errorf("ClassifyIntent(%q) should always carry a playbook", c.message)
			}
			if c.want == IntentAmbiguous && !got.Ambiguous {
				t.Errorf("ClassifyIntent(%q) should be flagged ambiguous", c.message)
			}
		})
	}
}

// 归因关键词应优先于趋势（同句含「下降」+「为什么」时判为归因）。
func TestClassifyIntent_AttributionBeatsTrend(t *testing.T) {
	got := ClassifyIntent("为什么最近销售额一直在下降")
	if got.Intent != IntentAttribution {
		t.Errorf("expected attribution to win over trend, got %v", got.Intent)
	}
}

// 路由 Hook 应把对应 playbook 注入到用户消息之前，且保留原始问题。
func TestIntentRoutingHook_InjectsPlaybook(t *testing.T) {
	// nil 分类模型 → 走词法兜底，判定确定可测。
	hook := intentRoutingHook(nil)
	msg := "最近半年的销售额趋势如何"
	_, routed, err := hook(context.Background(), msg)
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if !strings.Contains(routed, "趋势分析") {
		t.Errorf("routed message should contain trend playbook, got: %s", routed)
	}
	if !strings.Contains(routed, msg) {
		t.Errorf("routed message must preserve the original question")
	}
	if !strings.HasPrefix(routed, playbookTrend) {
		t.Errorf("playbook should be prepended before the question")
	}
}

// LLM 分类：合法输出（含带前后缀）应正确解析并映射到 playbook 与 ambiguous 标注。
func TestClassifyIntentLLM_Parses(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		want      Intent
		ambiguous bool
	}{
		{"精确", "attribution", IntentAttribution, false},
		{"带空白", "  trend\n", IntentTrend, false},
		{"带前缀", "intent: report", IntentReport, false},
		{"中文夹带", "归因(attribution)", IntentAttribution, false},
		{"歧义置标注", "ambiguous", IntentAmbiguous, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ClassifyIntentLLM(context.Background(), &stubModel{content: c.content}, "随便一句")
			if !ok {
				t.Fatalf("expected ok for %q", c.content)
			}
			if got.Intent != c.want {
				t.Errorf("ClassifyIntentLLM(%q).Intent = %v, want %v", c.content, got.Intent, c.want)
			}
			if got.Playbook == "" {
				t.Errorf("decision should carry a playbook")
			}
			if got.Ambiguous != c.ambiguous {
				t.Errorf("Ambiguous = %v, want %v", got.Ambiguous, c.ambiguous)
			}
		})
	}
}

// LLM 分类：模型缺失/出错/输出非法时返回 ok=false，交由词法兜底。
func TestClassifyIntentLLM_FallbackSignals(t *testing.T) {
	if _, ok := ClassifyIntentLLM(context.Background(), nil, "查询订单"); ok {
		t.Errorf("nil model should return ok=false")
	}
	if _, ok := ClassifyIntentLLM(context.Background(), &stubModel{err: errors.New("boom")}, "查询订单"); ok {
		t.Errorf("model error should return ok=false")
	}
	if _, ok := ClassifyIntentLLM(context.Background(), &stubModel{content: "不知道"}, "查询订单"); ok {
		t.Errorf("unparseable output should return ok=false")
	}
	if _, ok := ClassifyIntentLLM(context.Background(), &stubModel{content: "trend"}, "   "); ok {
		t.Errorf("empty message should return ok=false")
	}
}

// 路由 Hook：LLM 命中时应注入 LLM 判定对应的 playbook（此处 attribution）。
func TestIntentRoutingHook_UsesLLM(t *testing.T) {
	hook := intentRoutingHook(&stubModel{content: "attribution"})
	// 消息本身词法上像「取数」，验证走的是 LLM 判定而非词法。
	msg := "查一下这个指标"
	_, routed, err := hook(context.Background(), msg)
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if !strings.HasPrefix(routed, playbookAttribution) {
		t.Errorf("routed message should use LLM-classified attribution playbook, got: %s", routed)
	}
	if !strings.Contains(routed, msg) {
		t.Errorf("routed message must preserve the original question")
	}
}

// 路由 Hook：LLM 输出非法时回退词法（此消息词法判为趋势）。
func TestIntentRoutingHook_FallsBackToLexical(t *testing.T) {
	hook := intentRoutingHook(&stubModel{content: "garbage"})
	_, routed, err := hook(context.Background(), "最近半年的销售额趋势如何")
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if !strings.HasPrefix(routed, playbookTrend) {
		t.Errorf("should fall back to lexical trend playbook, got: %s", routed)
	}
}

func TestPlaybookFor_AllIntents(t *testing.T) {
	for _, in := range []Intent{IntentFetch, IntentTrend, IntentAttribution, IntentReport, IntentGeneral, IntentAmbiguous} {
		if playbookFor(in) == "" {
			t.Errorf("playbookFor(%v) is empty", in)
		}
	}
}

// 意图 → data_analysis 命名能力的显式映射（任务 4a.1：路由驱动分析编排）。
func TestCapabilityFor(t *testing.T) {
	cases := []struct {
		intent Intent
		want   string
	}{
		{IntentTrend, "trend"},
		{IntentAttribution, "attribution"},
		{IntentReport, "report"},
		{IntentFetch, ""},
		{IntentGeneral, ""},
		{IntentAmbiguous, ""},
	}
	for _, c := range cases {
		if got := CapabilityFor(c.intent); got != c.want {
			t.Errorf("CapabilityFor(%v) = %q, want %q", c.intent, got, c.want)
		}
	}
}

// 归因 playbook 必须显式点名 analysis_type=attribution 与 result_id 契约。
func TestPlaybookAttribution_NamesCapability(t *testing.T) {
	for _, want := range []string{"analysis_type=attribution", "result_id", "value_col", "period_col", "confidence", "drill_down"} {
		if !strings.Contains(playbookAttribution, want) {
			t.Errorf("playbookAttribution should mention %q", want)
		}
	}
}
