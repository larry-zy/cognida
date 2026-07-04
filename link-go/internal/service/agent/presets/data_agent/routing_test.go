package dataagent

import (
	"context"
	"strings"
	"testing"
)

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
	hook := intentRoutingHook()
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
