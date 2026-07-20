// Phase 7 任务 8.7：委派信封契约、上下文防火墙、循环/深度/scope 护栏、
// 只回传 handle、并行 fan-out 并发上限与失败独立性的单元测试。
package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainagent "link/internal/model/agent"
)

// fakeCollabAgent 是可观测的 Agent 桩：记录收到的消息与生效的工具门策略，
// 支持并发计数（并发上限断言）与按消息子串注入失败。
type fakeCollabAgent struct {
	name    string
	reply   string
	failSub string        // 消息含该子串时返回错误
	delay   time.Duration // 模拟耗时（驱动并发重叠）

	// 子代理内部真实执行痕迹：用于验证委派边界把工具轨迹/用量冒泡到父层。
	toolNames  []string
	tokens     int
	iterations int

	mu       sync.Mutex
	messages []string
	scopes   []string

	cur    int32
	maxCur int32
}

func (f *fakeCollabAgent) Chat(ctx context.Context, message string) (*Response, error) {
	c := atomic.AddInt32(&f.cur, 1)
	for {
		m := atomic.LoadInt32(&f.maxCur)
		if c <= m || atomic.CompareAndSwapInt32(&f.maxCur, m, c) {
			break
		}
	}
	defer atomic.AddInt32(&f.cur, -1)

	if f.delay > 0 {
		time.Sleep(f.delay)
	}

	f.mu.Lock()
	f.messages = append(f.messages, message)
	if p := ToolPolicyFromContext(ctx); p != nil {
		f.scopes = append(f.scopes, p.Scope)
	} else {
		f.scopes = append(f.scopes, "")
	}
	f.mu.Unlock()

	if f.failSub != "" && strings.Contains(message, f.failSub) {
		return nil, fmt.Errorf("injected failure")
	}
	calls := make([]*ToolCall, 0, len(f.toolNames))
	for _, n := range f.toolNames {
		calls = append(calls, &ToolCall{Name: n})
	}
	return &Response{
		Content:   f.reply,
		ToolCalls: calls,
		Metadata: map[string]interface{}{
			"tokens_used": f.tokens,
			"iterations":  f.iterations,
		},
	}, nil
}

func (f *fakeCollabAgent) Stream(ctx context.Context, message string) (<-chan *Chunk, error) {
	ch := make(chan *Chunk)
	close(ch)
	return ch, nil
}

func (f *fakeCollabAgent) Name() string { return f.name }

// newDelegateTestRegistry 注册一个隔离模式 Worker 与一个摘要模式 Reporter。
func newDelegateTestRegistry(worker, reporter *fakeCollabAgent) *CollaborationRegistry {
	registry := NewCollaborationRegistry()
	registry.RegisterGoverned("Worker", worker, "隔离取数工人",
		&AgentGovernance{Purpose: "取数", DataScope: "只读", Tools: []string{"sql_execute"}, RiskClass: ScopeRead},
		domainagent.ContextModeIsolated)
	if reporter != nil {
		registry.RegisterGoverned("Reporter", reporter, "摘要汇报者",
			&AgentGovernance{Purpose: "汇报", DataScope: "只读", RiskClass: ScopeRead},
			domainagent.ContextModeSummary)
	}
	return registry
}

func readEnvelope(agentName, goal, scope string) *DelegationEnvelope {
	return &DelegationEnvelope{
		AgentName:   agentName,
		Goal:        goal,
		Constraints: DelegationConstraints{Scope: scope},
	}
}

// TestExecuteDelegation_MissingGoalRejected 缺 goal 的委派必须被拒且留 rejected 痕。
func TestExecuteDelegation_MissingGoalRejected(t *testing.T) {
	worker := &fakeCollabAgent{name: "Worker", reply: "ok"}
	registry := newDelegateTestRegistry(worker, nil)

	var recorded []DelegationRecord
	SetDelegationRecorder(func(_ context.Context, rec DelegationRecord) {
		recorded = append(recorded, rec)
	})
	defer SetDelegationRecorder(nil)

	env := &DelegationEnvelope{AgentName: "Worker", Goal: "  ", Constraints: DelegationConstraints{Scope: ScopeRead}}
	_, err := executeDelegation(context.Background(), registry, env)
	if err == nil || !strings.Contains(err.Error(), "goal") {
		t.Fatalf("缺 goal 未被拒绝: err=%v", err)
	}
	if len(worker.messages) != 0 {
		t.Fatalf("被拒委派不应触达子代理，实际收到 %d 条", len(worker.messages))
	}
	if len(recorded) != 1 || recorded[0].Status != delegationStatusRejected {
		t.Fatalf("被拒委派应留 rejected 痕: %+v", recorded)
	}
}

// TestExecuteDelegation_MissingScopeRejected 缺 constraints.scope 不以默认值放行。
func TestExecuteDelegation_MissingScopeRejected(t *testing.T) {
	worker := &fakeCollabAgent{name: "Worker", reply: "ok"}
	registry := newDelegateTestRegistry(worker, nil)

	env := &DelegationEnvelope{AgentName: "Worker", Goal: "查销量"}
	_, err := executeDelegation(context.Background(), registry, env)
	if err == nil || !strings.Contains(err.Error(), "constraints.scope") {
		t.Fatalf("缺 scope 未被拒绝: err=%v", err)
	}
	if len(worker.messages) != 0 {
		t.Fatalf("被拒委派不应触达子代理")
	}
}

// TestExecuteDelegation_ScopeEscalationRejected 委派授予不得超过指挥官自身 scope。
func TestExecuteDelegation_ScopeEscalationRejected(t *testing.T) {
	worker := &fakeCollabAgent{name: "Worker", reply: "ok"}
	registry := newDelegateTestRegistry(worker, nil)

	ctx := WithToolPolicy(context.Background(), &ToolPolicy{Scope: ScopeRead})
	_, err := executeDelegation(ctx, registry, readEnvelope("Worker", "改库", ScopeWrite))
	if err == nil || !strings.Contains(err.Error(), "越权") {
		t.Fatalf("scope 越权未被拒绝: err=%v", err)
	}

	// 同级授予应放行
	out, err := executeDelegation(ctx, registry, readEnvelope("Worker", "查销量", ScopeRead))
	if err != nil || out != "ok" {
		t.Fatalf("同级 scope 委派应放行: out=%q err=%v", out, err)
	}
}

// TestExecuteDelegation_IsolatedContextNoLeak 隔离模式子代理只见信封，
// 不泄漏指挥官原始问题/摘要；摘要模式子代理可见协作上下文。
func TestExecuteDelegation_IsolatedContextNoLeak(t *testing.T) {
	worker := &fakeCollabAgent{name: "Worker", reply: "ok"}
	reporter := &fakeCollabAgent{name: "Reporter", reply: "ok"}
	registry := newDelegateTestRegistry(worker, reporter)

	collab := domainagent.NewCollaborationContext("s1", 1, "机密原始问题")
	collab.Summary = "机密对话摘要"
	ctx := domainagent.WithCollaborationContext(context.Background(), collab)

	if _, err := executeDelegation(ctx, registry, readEnvelope("Worker", "查上月销量", ScopeRead)); err != nil {
		t.Fatalf("隔离委派失败: %v", err)
	}
	if _, err := executeDelegation(ctx, registry, readEnvelope("Reporter", "汇总结论", ScopeRead)); err != nil {
		t.Fatalf("摘要委派失败: %v", err)
	}

	isolatedMsg := worker.messages[0]
	if strings.Contains(isolatedMsg, "机密") {
		t.Fatalf("隔离模式泄漏了指挥官上下文:\n%s", isolatedMsg)
	}
	if !strings.Contains(isolatedMsg, "查上月销量") || !strings.Contains(isolatedMsg, "## 委派任务") {
		t.Fatalf("隔离模式应携带委派信封内容:\n%s", isolatedMsg)
	}

	summaryMsg := reporter.messages[0]
	if !strings.Contains(summaryMsg, "机密原始问题") || !strings.Contains(summaryMsg, "机密对话摘要") {
		t.Fatalf("摘要模式应携带协作上下文:\n%s", summaryMsg)
	}
}

// TestExecuteDelegation_HandleOnlyReturnAndScopedGrant 只回传子代理最终内容，
// 且子代理在委派 ctx 中拿到信封 scope 的工具门策略（每次委派授予）。
func TestExecuteDelegation_HandleOnlyReturnAndScopedGrant(t *testing.T) {
	worker := &fakeCollabAgent{name: "Worker", reply: "result_id=res-42；摘要：上月销量环比 +12%"}
	registry := newDelegateTestRegistry(worker, nil)

	collab := domainagent.NewCollaborationContext("s1", 1, "原始问题")
	ctx := domainagent.WithCollaborationContext(context.Background(), collab)

	out, err := executeDelegation(ctx, registry, readEnvelope("Worker", "查销量", ScopeRead))
	if err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	if out != worker.reply {
		t.Fatalf("应只回传子代理最终内容: got=%q", out)
	}
	if worker.scopes[0] != ScopeRead {
		t.Fatalf("子代理应拿到信封 scope 的工具门策略: got=%q", worker.scopes[0])
	}
	// 结果句柄落回指挥官协作上下文
	if res, ok := collab.GetResult("Worker"); !ok || res.Content != worker.reply {
		t.Fatalf("委派结果应回写指挥官协作上下文: %+v ok=%v", res, ok)
	}
	// 指挥官自身链路不受子委派污染（路径式循环语义：可再次委派同一子代理）
	if _, err := executeDelegation(ctx, registry, readEnvelope("Worker", "再查一次", ScopeRead)); err != nil {
		t.Fatalf("对同一子代理的串行再委派应放行: %v", err)
	}
}

// TestExecuteDelegation_TrajectoryBubblesUp 验证委派边界把子代理内部真实工具轨迹与
// 运行时用量冒泡到父层痕迹——回归 data_agent 等指挥官型 agent 委派后，子代理真实执行的
// get_schema/sql_execute 与 token 消耗被上下文防火墙吞掉、Agent 评测工具/用量指标失真的缺陷。
// 回传给 LLM 的内容仍只是子代理最终摘要（防火墙不变）。
func TestExecuteDelegation_TrajectoryBubblesUp(t *testing.T) {
	worker := &fakeCollabAgent{
		name:       "Worker",
		reply:      "result_id=res-9；摘要：上月销量 12345",
		toolNames:  []string{"get_schema", "sql_execute"},
		tokens:     420,
		iterations: 3,
	}
	registry := newDelegateTestRegistry(worker, nil)

	// 模拟指挥官 run 安装的委派痕迹累积器
	trace := &delegationTrace{}
	ctx := withDelegationTrace(context.Background(), trace)

	out, err := executeDelegation(ctx, registry, readEnvelope("Worker", "查销量", ScopeRead))
	if err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	// 防火墙不变：回传仍只是子代理最终内容，不含内部工具往返
	if out != worker.reply {
		t.Fatalf("回传应只是子代理最终内容: got=%q", out)
	}

	// 子代理真实工具轨迹与用量已冒泡到父层痕迹
	tcs, tok, iters := trace.drain()
	got := make([]string, 0, len(tcs))
	for _, tc := range tcs {
		got = append(got, tc.Name)
	}
	if len(got) != 2 || got[0] != "get_schema" || got[1] != "sql_execute" {
		t.Fatalf("子代理工具轨迹未冒泡: %v", got)
	}
	if tok != 420 || iters != 3 {
		t.Fatalf("子代理运行时用量未冒泡: tokens=%d iterations=%d", tok, iters)
	}

	// 无痕迹累积器时（如非评测直连路径）不应 panic：nil 接收者按无操作
	if _, err := executeDelegation(context.Background(), registry, readEnvelope("Worker", "再查", ScopeRead)); err != nil {
		t.Fatalf("无痕迹累积器时委派应正常: %v", err)
	}
}

// TestExecuteDelegation_CyclicBlocked 委派路径上已有目标代理时拦截（A→B→A）。
func TestExecuteDelegation_CyclicBlocked(t *testing.T) {
	worker := &fakeCollabAgent{name: "Worker", reply: "ok"}
	registry := newDelegateTestRegistry(worker, nil)

	collab := domainagent.NewCollaborationContext("s1", 1, "q")
	collab.AddDelegate("Worker") // 模拟当前执行者已在 Worker 的委派路径内
	ctx := domainagent.WithCollaborationContext(context.Background(), collab)

	_, err := executeDelegation(ctx, registry, readEnvelope("Worker", "回头找自己", ScopeRead))
	if err == nil || !strings.Contains(err.Error(), "循环") {
		t.Fatalf("循环委派未被拦截: err=%v", err)
	}
}

// TestExecuteDelegation_DepthExceeded 超过 MaxDepth 拒绝继续下钻。
func TestExecuteDelegation_DepthExceeded(t *testing.T) {
	worker := &fakeCollabAgent{name: "Worker", reply: "ok"}
	registry := newDelegateTestRegistry(worker, nil)

	collab := domainagent.NewCollaborationContext("s1", 1, "q")
	collab.MaxDepth = 1
	collab.AddDelegate("Other")
	ctx := domainagent.WithCollaborationContext(context.Background(), collab)

	_, err := executeDelegation(ctx, registry, readEnvelope("Worker", "更深一层", ScopeRead))
	if err == nil || !strings.Contains(err.Error(), "深度") {
		t.Fatalf("超深度委派未被拦截: err=%v", err)
	}
}

// parallelOutput 并行委派工具输出的断言载体。
type parallelOutput struct {
	Total     int                        `json:"total"`
	Succeeded int                        `json:"succeeded"`
	Failed    int                        `json:"failed"`
	Results   []parallelDelegationResult `json:"results"`
	Hint      string                     `json:"hint"`
}

func runParallel(t *testing.T, ctx context.Context, registry *CollaborationRegistry, maxConc int, envs []DelegationEnvelope) *parallelOutput {
	t.Helper()
	pt := NewParallelDelegateTool(registry, maxConc)
	argsJSON, err := json.Marshal(map[string]interface{}{"delegations": envs})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	raw, err := pt.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		t.Fatalf("并行委派执行失败: %v", err)
	}
	var out parallelOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("解析并行输出失败: %v\n%s", err, raw)
	}
	return &out
}

// TestParallelDelegate_ConcurrencyCap 并行 fan-out 受并发上限约束。
func TestParallelDelegate_ConcurrencyCap(t *testing.T) {
	worker := &fakeCollabAgent{name: "Worker", reply: "ok", delay: 30 * time.Millisecond}
	registry := newDelegateTestRegistry(worker, nil)

	envs := make([]DelegationEnvelope, 6)
	for i := range envs {
		envs[i] = *readEnvelope("Worker", fmt.Sprintf("独立子任务-%d", i), ScopeRead)
	}
	out := runParallel(t, context.Background(), registry, 2, envs)

	if out.Total != 6 || out.Succeeded != 6 {
		t.Fatalf("并行委派应全部成功: %+v", out)
	}
	if got := atomic.LoadInt32(&worker.maxCur); got > 2 {
		t.Fatalf("并发峰值 %d 超过上限 2", got)
	}
}

// TestParallelDelegate_FailureIndependence 单项失败不牵连并行兄弟，
// 失败项标记明确且给出独立重试提示；成功项结果回写父上下文。
func TestParallelDelegate_FailureIndependence(t *testing.T) {
	worker := &fakeCollabAgent{name: "Worker", reply: "ok", failSub: "注定失败"}
	registry := newDelegateTestRegistry(worker, nil)

	collab := domainagent.NewCollaborationContext("s1", 1, "q")
	ctx := domainagent.WithCollaborationContext(context.Background(), collab)

	envs := []DelegationEnvelope{
		*readEnvelope("Worker", "子任务-A", ScopeRead),
		*readEnvelope("Worker", "注定失败的子任务", ScopeRead),
		*readEnvelope("Worker", "子任务-C", ScopeRead),
	}
	out := runParallel(t, ctx, registry, 3, envs)

	if out.Succeeded != 2 || out.Failed != 1 {
		t.Fatalf("失败应独立: %+v", out)
	}
	if out.Hint == "" || !strings.Contains(out.Hint, "重试") {
		t.Fatalf("失败时应给出独立重试提示: %+v", out)
	}
	for _, r := range out.Results {
		if r.Goal == "注定失败的子任务" {
			if r.Status != "failed" || r.Error == "" {
				t.Fatalf("失败项标记不明确: %+v", r)
			}
		} else if r.Status != "ok" {
			t.Fatalf("成功项被失败牵连: %+v", r)
		}
	}
	if _, ok := collab.GetResult("Worker"); !ok {
		t.Fatalf("成功项结果应回写父协作上下文")
	}
	// 并行兄弟不追加父链：后续对同一子代理的委派不被误伤
	if _, err := executeDelegation(ctx, registry, readEnvelope("Worker", "重试失败项", ScopeRead)); err != nil {
		t.Fatalf("失败项独立重试应放行: %v", err)
	}
}

// TestDelegateTool_EnvelopeContract delegate_to_agent 以信封为参数契约，
// 非法 JSON 与缺字段均拒绝。
func TestDelegateTool_EnvelopeContract(t *testing.T) {
	worker := &fakeCollabAgent{name: "Worker", reply: "ok"}
	registry := newDelegateTestRegistry(worker, nil)
	dt := NewDelegateTool(registry)

	if _, err := dt.InvokableRun(context.Background(), "{not json"); err == nil {
		t.Fatalf("非法 JSON 应报错")
	}
	if _, err := dt.InvokableRun(context.Background(),
		`{"agent_name":"Worker","goal":"查销量"}`); err == nil || !strings.Contains(err.Error(), "constraints.scope") {
		t.Fatalf("缺 scope 应被拒: %v", err)
	}
	out, err := dt.InvokableRun(context.Background(),
		`{"agent_name":"Worker","goal":"查销量","constraints":{"scope":"read","max_rows":100}}`)
	if err != nil || out != "ok" {
		t.Fatalf("合法信封应放行: out=%q err=%v", out, err)
	}
}
