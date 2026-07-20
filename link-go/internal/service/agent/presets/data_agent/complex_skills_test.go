package dataagent

import (
	"context"
	"strings"
	"testing"

	domainagent "link/internal/model/agent"
	infraagent "link/internal/service/agent/framework"
)

// fakeSubAgent 是可观测的子代理桩：按名固定回一段紧凑结论，记录收到的委派消息，
// 可选按消息子串注入失败（模拟子代理战术修复后仍搞不定）。
type fakeSubAgent struct {
	name     string
	reply    string
	failSub  string
	messages []string
}

func (f *fakeSubAgent) Chat(_ context.Context, message string) (*infraagent.Response, error) {
	f.messages = append(f.messages, message)
	if f.failSub != "" && strings.Contains(message, f.failSub) {
		return nil, errFakeFail
	}
	return &infraagent.Response{Content: f.reply}, nil
}

func (f *fakeSubAgent) Stream(context.Context, string) (<-chan *infraagent.Chunk, error) {
	return nil, errFakeFail
}

func (f *fakeSubAgent) Name() string { return f.name }

var errFakeFail = &fakeErr{}

type fakeErr struct{}

func (*fakeErr) Error() string { return "injected sub-agent failure" }

// registryWith 构造一个注入了指定子代理桩的协作注册表。
func registryWith(agents ...*fakeSubAgent) *infraagent.CollaborationRegistry {
	reg := infraagent.NewCollaborationRegistry()
	for _, a := range agents {
		reg.RegisterGoverned(a.name, a, a.name+" 桩", &infraagent.AgentGovernance{
			Purpose:   "test",
			RiskClass: infraagent.ScopeRead,
		}, domainagent.ContextModeIsolated)
	}
	return reg
}

// ctxWith 注入协作注册表 + 一个新的协作上下文（供 IsCyclic/Clone 生效）。
func ctxWith(reg *infraagent.CollaborationRegistry, chain ...string) context.Context {
	ctx := infraagent.WithCollaborationRegistry(context.Background(), reg)
	cc := domainagent.NewCollaborationContext("s", 1, "原始问题")
	for _, name := range chain {
		cc.AddDelegate(name)
	}
	return domainagent.WithCollaborationContext(ctx, cc)
}

// TestAttribution_OnlySummaryReturned 校验：多维归因 handler 串行编排 SQLAuthor→Analysis，
// 只回传 Analysis 的最终结论；SQLAuthor 的内部结果不出现在最终回传里（内部往返不回灌）。
func TestAttribution_OnlySummaryReturned(t *testing.T) {
	sql := &fakeSubAgent{name: "SQLAuthor", reply: "SQL_INTERNAL result_id=r-777"}
	analysis := &fakeSubAgent{name: "Analysis", reply: "归因结论：渠道 A 贡献 60%"}
	ctx := ctxWith(registryWith(sql, analysis))

	out, err := attributionHandler(ctx, `{"task":"GMV 环比下滑归因"}`)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out != analysis.reply {
		t.Fatalf("应只回传 Analysis 结论, got %q", out)
	}
	if strings.Contains(out, "SQL_INTERNAL") {
		t.Fatalf("SQLAuthor 内部结果不应回灌主循环: %q", out)
	}
	// 校验编排确有发生：两个子代理各被委派一次，且 SQLAuthor 结果被作为输入携给 Analysis。
	if len(sql.messages) != 1 || len(analysis.messages) != 1 {
		t.Fatalf("应各委派一次: sql=%d analysis=%d", len(sql.messages), len(analysis.messages))
	}
	if !strings.Contains(analysis.messages[0], "result_id=r-777") {
		t.Fatalf("SQLAuthor 结果应作为输入传给 Analysis: %q", analysis.messages[0])
	}
}

// TestAttribution_FetchFailurePropagates 校验：取数子代理战术修复后仍失败时，
// handler 返回错误（交主 agent/编排层做战略重规划，D5 护栏在主层计数），不静默吞掉。
func TestAttribution_FetchFailurePropagates(t *testing.T) {
	sql := &fakeSubAgent{name: "SQLAuthor", failSub: "取数"}
	analysis := &fakeSubAgent{name: "Analysis", reply: "不应到达"}
	ctx := ctxWith(registryWith(sql, analysis))

	_, err := attributionHandler(ctx, `{"task":"归因"}`)
	if err == nil {
		t.Fatal("取数失败应向上传递错误")
	}
	if len(analysis.messages) != 0 {
		t.Fatal("取数失败后不应再委派 Analysis")
	}
}

// TestAttribution_CyclicBlocked 校验：委派链已含目标子代理时，IsCyclic 护栏拦截，
// handler 直接失败（inline 复用同一委派内核护栏，不绕过）。
func TestAttribution_CyclicBlocked(t *testing.T) {
	sql := &fakeSubAgent{name: "SQLAuthor", reply: "x"}
	analysis := &fakeSubAgent{name: "Analysis", reply: "y"}
	ctx := ctxWith(registryWith(sql, analysis), "SQLAuthor") // 链中已有 SQLAuthor

	_, err := attributionHandler(ctx, `{"task":"归因"}`)
	if err == nil {
		t.Fatal("循环委派应被 IsCyclic 拦截")
	}
	if !strings.Contains(err.Error(), "循环") {
		t.Fatalf("应为循环委派错误: %v", err)
	}
}

// TestAttribution_NoRegistryDegrades 校验：未注入注册表（如简单任务的裸 ctx）时，
// handler 降级为明确错误而非 panic。
func TestAttribution_NoRegistryDegrades(t *testing.T) {
	_, err := attributionHandler(context.Background(), `{"task":"归因"}`)
	if err == nil {
		t.Fatal("无注册表应报错")
	}
}

// TestBusinessReport_ReadOnlyByDefault 校验：无导出/落库意图时，经营报告只编排 Report（只读），
// 不触发 Operation 写子代理。
func TestBusinessReport_ReadOnlyByDefault(t *testing.T) {
	report := &fakeSubAgent{name: "Report", reply: "## 经营报告\n增长稳健"}
	op := &fakeSubAgent{name: "Operation", reply: "写句柄"}
	ctx := ctxWith(registryWith(report, op))

	out, err := businessReportHandler(ctx, `{"task":"生成上月经营报告"}`)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out != report.reply {
		t.Fatalf("只读报告应原样回传: %q", out)
	}
	if len(op.messages) != 0 {
		t.Fatal("无落库意图不应触发 Operation 写")
	}
}

// TestBusinessReport_PersistIntentDelegatesOperation 校验：显式要求导出/落库时，
// 报告产出后下沉给 Operation 做写，且报告本体仍被保留回传。
func TestBusinessReport_PersistIntentDelegatesOperation(t *testing.T) {
	report := &fakeSubAgent{name: "Report", reply: "报告正文"}
	op := &fakeSubAgent{name: "Operation", reply: "已落库 tbl_report"}
	ctx := ctxWith(registryWith(report, op))

	out, err := businessReportHandler(ctx, `{"task":"生成经营报告并导出到表"}`)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(op.messages) != 1 {
		t.Fatalf("有导出意图应委派 Operation 一次, got %d", len(op.messages))
	}
	if !strings.Contains(out, "报告正文") || !strings.Contains(out, "已落库") {
		t.Fatalf("应同时含报告与落库句柄: %q", out)
	}
}

// TestSkillTaskFromInput 校验入参解析：JSON 提取 task/question，非 JSON 退化为原文。
func TestSkillTaskFromInput(t *testing.T) {
	cases := map[string]string{
		`{"skill_name":"x","task":" 归因 "}`: "归因",
		`{"question":"报告"}`:               "报告",
		`纯文本目标`:                           "纯文本目标",
	}
	for in, want := range cases {
		if got := skillTaskFromInput(in); got != want {
			t.Fatalf("skillTaskFromInput(%q)=%q want %q", in, got, want)
		}
	}
}
