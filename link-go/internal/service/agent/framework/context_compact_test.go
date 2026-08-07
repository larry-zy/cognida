package framework

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	ctxeng "link/internal/service/agent/context"
)

// mkAssistantToolCall 造一条发起工具调用的 assistant 消息（一条 tool_call）。
func mkAssistantToolCall(id, content string) *schema.Message {
	return schema.AssistantMessage(content, []schema.ToolCall{{
		ID:       id,
		Function: schema.FunctionCall{Name: "sql_execute", Arguments: "{}"},
	}})
}

// firstNonSystemAfterSummary 返回折叠后除 messages[0](系统提示) 与紧随其后摘要外的首条业务消息。
// 不变量校验：该条不得是 tool（否则 assistant.tool_calls↔tool 的 1:1 配对被破坏 → LLM 400）。
func assertNoOrphanTool(t *testing.T, msgs []*schema.Message) {
	t.Helper()
	// 逐条扫描：每遇到 tool 消息，其前面必须存在一条带同 ID tool_call 的 assistant。
	seen := map[string]bool{}
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			seen[tc.ID] = true
		}
		if m.Role == schema.Tool {
			if !seen[m.ToolCallID] {
				t.Fatalf("发现孤儿 tool 消息（ID=%q 无前置 assistant.tool_calls）", m.ToolCallID)
			}
		}
	}
}

// TestNormalizeContext_CapsOversizedSingleMessage 验证 ①：单条超 maxMessageTokens 被压缩且保 result_id。
func TestNormalizeContext_CapsOversizedSingleMessage(t *testing.T) {
	a := &agentImpl{maxMessageTokens: 50}
	c := ctxeng.ApproxTokenCounter{}
	long := strings.Repeat("这条工具观察非常长内容明细", 300) + " 见 rs_bigresult"
	msgs := []*schema.Message{
		schema.SystemMessage("SYS"),
		schema.UserMessage("问题"),
		schema.ToolMessage(long, "call-1"),
	}
	got := a.normalizeContext(context.Background(), msgs)
	if c.Count(got[2].Content) > 50+10 {
		t.Errorf("超限单条应被压到 ~50 token, got %d", c.Count(got[2].Content))
	}
	if !strings.Contains(got[2].Content, "rs_bigresult") {
		t.Errorf("压缩后必须保住 result_id rs_bigresult, got 末尾: %q", got[2].Content)
	}
	if got[0].Content != "SYS" {
		t.Errorf("系统提示 Pinned 不应被压缩")
	}
}

// TestNormalizeContext_NoopWhenUnconfigured 三阈值都为 0 时逐字节零回归。
func TestNormalizeContext_NoopWhenUnconfigured(t *testing.T) {
	a := &agentImpl{}
	msgs := []*schema.Message{
		schema.SystemMessage("SYS"),
		schema.UserMessage(strings.Repeat("很长的问题", 1000)),
	}
	got := a.normalizeContext(context.Background(), msgs)
	if len(got) != 2 || got[1].Content != msgs[1].Content {
		t.Errorf("未配置阈值应原样返回")
	}
}

// TestFoldOlderMessages_MaskingPrimaryPreservesPairingAndResultID 验证 ③ Level A（观察屏蔽为主）：
// 累计超触发线时，先屏蔽早期的大工具观察（保 result_id）、逐字保留全部推理/动作与近期尾巴，
// 屏蔽后已回到触发线下则不再摘要——保住完整决策链、消息数不变、tool 配对不破坏。
func TestFoldOlderMessages_MaskingPrimaryPreservesPairingAndResultID(t *testing.T) {
	a := &agentImpl{compactTrigger: 4000, compactTarget: 200}
	c := ctxeng.ApproxTokenCounter{}

	// 构造多轮 react：user → (assistant tool_call, tool)×N。早期工具观察很大以触发折叠。
	longObs := func(rid string) string {
		return strings.Repeat("这一轮的工具观察结果明细很长很长很长", 120) + " 结果 " + rid
	}
	msgs := []*schema.Message{schema.SystemMessage("SYS-PROMPT")}
	msgs = append(msgs, schema.UserMessage("最初的问题"))
	// 早期 3 轮（其工具观察会被屏蔽），各埋一个 result_id。
	for i, rid := range []string{"rs_early1", "rs_early2", "rs_early3"} {
		msgs = append(msgs, mkAssistantToolCall("c-early-"+string(rune('a'+i)), "调用工具推理"+rid))
		msgs = append(msgs, schema.ToolMessage(longObs(rid), "c-early-"+string(rune('a'+i))))
	}
	// 近期 1 轮（应逐字保留）。
	msgs = append(msgs, mkAssistantToolCall("c-recent", "最近一轮调用"))
	msgs = append(msgs, schema.ToolMessage("最近一轮的简短观察 rs_recent", "c-recent"))

	before := totalMessageTokens(msgs, c)
	if before < a.compactTrigger {
		t.Fatalf("构造失败：总量 %d 应 ≥ trigger %d", before, a.compactTrigger)
	}

	got := a.normalizeContext(context.Background(), msgs)

	// 系统提示 Pinned 保留在首位。
	if got[0].Role != schema.System || got[0].Content != "SYS-PROMPT" {
		t.Fatalf("首条应为原系统提示, got (%s,%.20q)", got[0].Role, got[0].Content)
	}
	// 观察屏蔽为主：消息数不变（仅缩小 tool 内容，不折成摘要）。
	if len(got) != len(msgs) {
		t.Fatalf("观察屏蔽应保持消息数不变, got %d want %d", len(got), len(msgs))
	}
	// 无摘要系统消息（屏蔽已足够，不触发 Level B）。
	if got[1].Role == schema.System && strings.Contains(got[1].Content, "历史已压缩") {
		t.Fatalf("屏蔽已足够时不应出现摘要系统消息, got[1]=%.30q", got[1].Content)
	}
	// 配对不变量：折叠后无孤儿 tool。
	assertNoOrphanTool(t, got)
	// 早期工具观察已被屏蔽（大块原文消失、占位前缀出现），但 result_id 全部保真。
	for _, rid := range []string{"rs_early1", "rs_early2", "rs_early3"} {
		if !containsInAny(got, rid) {
			t.Errorf("屏蔽后必须保住早期 result_id %s", rid)
		}
	}
	if containsInAny(got, "这一轮的工具观察结果明细") {
		t.Errorf("早期大工具观察应被屏蔽掉大块原文")
	}
	// 推理/动作(assistant)逐字保留（决策链不丢）。
	if !containsInAny(got, "调用工具推理rs_early1") {
		t.Errorf("早期推理/动作应逐字保留（决策链）")
	}
	// 近期轮逐字保留（末条仍是最近观察）。
	last := got[len(got)-1]
	if last.Role != schema.Tool || !strings.Contains(last.Content, "rs_recent") {
		t.Errorf("近期轮应逐字保留在尾部, got (%s,%.30q)", last.Role, last.Content)
	}
	// 总量确有下降。
	if after := totalMessageTokens(got, c); after >= before {
		t.Errorf("屏蔽后总量(%d)应低于折叠前(%d)", after, before)
	}
}

// TestFoldOlderMessages_SummaryFallbackWhenReasoningHuge 验证 ③ Level B（摘要兜底）：
// 当推理/动作本身极长、仅屏蔽工具观察仍压不回触发线下时，退化为整轮抽取式摘要，
// 早期块折成一条摘要系统消息、result_id 保真、tool 配对不破坏。
func TestFoldOlderMessages_SummaryFallbackWhenReasoningHuge(t *testing.T) {
	a := &agentImpl{compactTrigger: 4000, compactTarget: 200}
	c := ctxeng.ApproxTokenCounter{}

	// 早期若干轮的 assistant 推理本身极长（屏蔽工具观察也压不下去）→ 逼出 Level B 摘要。
	hugeReason := strings.Repeat("冗长的推理链条分析论证反复展开", 220)
	msgs := []*schema.Message{schema.SystemMessage("SYS-PROMPT"), schema.UserMessage("最初的问题")}
	for i, rid := range []string{"rs_e1", "rs_e2", "rs_e3"} {
		msgs = append(msgs, mkAssistantToolCall("c-"+string(rune('a'+i)), hugeReason+" "+rid))
		msgs = append(msgs, schema.ToolMessage("观察 "+rid, "c-"+string(rune('a'+i))))
	}
	msgs = append(msgs, mkAssistantToolCall("c-recent", "最近一轮调用"))
	msgs = append(msgs, schema.ToolMessage("最近观察 rs_recent", "c-recent"))

	if totalMessageTokens(msgs, c) < a.compactTrigger {
		t.Fatalf("构造失败：应超 trigger")
	}

	got := a.normalizeContext(context.Background(), msgs)

	if got[0].Role != schema.System || got[0].Content != "SYS-PROMPT" {
		t.Fatalf("首条应为原系统提示, got (%s,%.20q)", got[0].Role, got[0].Content)
	}
	// 兜底触发：次条应为折叠摘要系统消息。
	if got[1].Role != schema.System || !strings.Contains(got[1].Content, "历史已压缩") {
		t.Fatalf("推理链极长应退化为摘要, got[1]=(%s,%.30q)", got[1].Role, got[1].Content)
	}
	assertNoOrphanTool(t, got)
	for _, rid := range []string{"rs_e1", "rs_e2", "rs_e3"} {
		if !strings.Contains(got[1].Content, rid) {
			t.Errorf("摘要必须保住早期 result_id %s", rid)
		}
	}
	last := got[len(got)-1]
	if last.Role != schema.Tool || !strings.Contains(last.Content, "rs_recent") {
		t.Errorf("近期轮应逐字保留在尾部, got (%s,%.30q)", last.Role, last.Content)
	}
}

// TestFoldOlderMessages_ReasoningWeightCountsAndReclaims 验证 DeepSeek thinking 账目修正：
// 思考挂在 ReasoningContent 字段（DeepSeek 契约强制回传、不可剥离）。即便 Content 很小，只要
// 累积思考超触发线也应触发 ③；且观察屏蔽碰不到 assistant 的 reasoning，只有 Level B 整轮折叠
// 能把它连同整轮移除回收。
func TestFoldOlderMessages_ReasoningWeightCountsAndReclaims(t *testing.T) {
	a := &agentImpl{compactTrigger: 4000, compactTarget: 200}
	c := ctxeng.ApproxTokenCounter{}

	// Content 都很短；把体积全塞进 ReasoningContent（模拟 thinking 默认开的中间工具轮）。
	hugeThinking := strings.Repeat("反复权衡的内部思考链条很长很长", 220)
	withReasoning := func(id, content, reasoning string) *schema.Message {
		m := mkAssistantToolCall(id, content)
		m.ReasoningContent = reasoning
		return m
	}
	msgs := []*schema.Message{schema.SystemMessage("SYS-PROMPT"), schema.UserMessage("最初的问题")}
	for i, rid := range []string{"rs_r1", "rs_r2", "rs_r3"} {
		msgs = append(msgs, withReasoning("c-"+string(rune('a'+i)), "简短动作 "+rid, hugeThinking))
		msgs = append(msgs, schema.ToolMessage("简短观察 "+rid, "c-"+string(rune('a'+i))))
	}
	msgs = append(msgs, mkAssistantToolCall("c-recent", "最近一轮调用"))
	msgs = append(msgs, schema.ToolMessage("最近观察 rs_recent", "c-recent"))

	// 账目修正的核心断言：只数 Content 会严重低估、判为无需压缩；计入 reasoning 才越过触发线。
	contentOnly := 0
	for _, m := range msgs {
		contentOnly += c.Count(m.Content)
	}
	if contentOnly >= a.compactTrigger {
		t.Fatalf("构造失败：Content-only 应远低于 trigger（才能证明是 reasoning 触发），got %d", contentOnly)
	}
	if totalMessageTokens(msgs, c) < a.compactTrigger {
		t.Fatalf("计入 reasoning 后应越过 trigger，got %d", totalMessageTokens(msgs, c))
	}

	before := totalMessageTokens(msgs, c)
	got := a.normalizeContext(context.Background(), msgs)

	// 只有整轮折叠能回收 reasoning：应出现摘要系统消息，且总量（含 reasoning）显著下降。
	if got[1].Role != schema.System || !strings.Contains(got[1].Content, "历史已压缩") {
		t.Fatalf("reasoning 堆积应经 Level B 折叠回收, got[1]=(%s,%.30q)", got[1].Role, got[1].Content)
	}
	assertNoOrphanTool(t, got)
	if after := totalMessageTokens(got, c); after >= before {
		t.Errorf("折叠后总量(%d, 含 reasoning)应低于折叠前(%d)", after, before)
	}
	// 早期轮 result_id 仍保真（在摘要里）。
	for _, rid := range []string{"rs_r1", "rs_r2", "rs_r3"} {
		if !strings.Contains(got[1].Content, rid) {
			t.Errorf("摘要必须保住早期 result_id %s", rid)
		}
	}
}

// TestNormalizeContext_ReasoningEvictionShedsOldThinkingKeepsRecent 验证 ③.5：
// 远在 128k 触发线之下（compactTrigger 极大、③ 不触发），只要「最近一轮之前」累计的 reasoning
// 超过 reasoningEvictTokens，就把旧轮整轮折走（陈旧思考随之离场），而最近一轮的 thinking 逐字保留
// （DeepSeek 契约本轮必须带）。同时保住早期 result_id、不产生孤儿 tool。
func TestNormalizeContext_ReasoningEvictionShedsOldThinkingKeepsRecent(t *testing.T) {
	// compactTrigger 设极大 → ③ 永不触发，隔离出「是 ③.5 而非 ③ 在动手」。
	a := &agentImpl{compactTrigger: 1_000_000, compactTarget: 200, reasoningEvictTokens: 500}
	c := ctxeng.ApproxTokenCounter{}

	oldThinking := strings.Repeat("旧轮里反复权衡的内部思考", 30) // 每轮一坨，攒几轮越过 500
	recentThinking := "最近一轮独有的思考指纹RECENTFINGERPRINT"
	withReasoning := func(id, content, reasoning string) *schema.Message {
		m := mkAssistantToolCall(id, content)
		m.ReasoningContent = reasoning
		return m
	}
	msgs := []*schema.Message{schema.SystemMessage("SYS-PROMPT"), schema.UserMessage("最初的问题")}
	for i, rid := range []string{"rs_r1", "rs_r2", "rs_r3"} {
		msgs = append(msgs, withReasoning("c-"+string(rune('a'+i)), "简短动作 "+rid, oldThinking))
		msgs = append(msgs, schema.ToolMessage("简短观察 "+rid, "c-"+string(rune('a'+i))))
	}
	msgs = append(msgs, withReasoning("c-recent", "最近一轮调用", recentThinking))
	msgs = append(msgs, schema.ToolMessage("最近观察 rs_recent", "c-recent"))

	// 构造前提：整段远低于 compactTrigger（证明 ③ 不会触发），但旧轮 reasoning 已越过剥离阈值。
	if totalMessageTokens(msgs, c) >= a.compactTrigger {
		t.Fatalf("构造失败：整段应远低于 compactTrigger（隔离 ③），got %d", totalMessageTokens(msgs, c))
	}
	recentCut := recentTailCut(msgs, 0, c)
	if oldReasoningTokens(msgs, recentCut, c) < a.reasoningEvictTokens {
		t.Fatalf("构造失败：旧轮累计 reasoning 应越过剥离阈值, got %d", oldReasoningTokens(msgs, recentCut, c))
	}

	got := a.normalizeContext(context.Background(), msgs)

	// 旧轮被整轮折成摘要系统消息置于 index 1。
	if got[1].Role != schema.System || !strings.Contains(got[1].Content, "历史已压缩") {
		t.Fatalf("旧轮应被 ③.5 整轮折走为摘要, got[1]=(%s,%.30q)", got[1].Role, got[1].Content)
	}
	assertNoOrphanTool(t, got)

	// 最近一轮的 thinking 逐字保留（本轮契约必须带）。
	keptRecent := false
	for _, m := range got {
		if strings.Contains(m.ReasoningContent, "RECENTFINGERPRINT") {
			keptRecent = true
		}
	}
	if !keptRecent {
		t.Errorf("最近一轮的 reasoning 必须逐字保留（本轮可带）")
	}

	// 旧轮的陈旧思考应已随整轮离场：折叠后累计 reasoning 显著回落到阈值下。
	if left := oldReasoningTokens(got, len(got), c); left >= a.reasoningEvictTokens {
		t.Errorf("旧思考应随整轮折走, 折叠后残留 reasoning=%d 应 < 阈值 %d", left, a.reasoningEvictTokens)
	}
	if containsInAnyReasoning(got, "旧轮里反复权衡") {
		t.Errorf("旧轮的 thinking 不应再出现在上下文中")
	}

	// 早期 result_id 仍在摘要里保真。
	for _, rid := range []string{"rs_r1", "rs_r2", "rs_r3"} {
		if !strings.Contains(got[1].Content, rid) {
			t.Errorf("摘要必须保住早期 result_id %s", rid)
		}
	}
}

// containsInAnyReasoning 报告 sub 是否出现在任一消息的 ReasoningContent 中。
func containsInAnyReasoning(msgs []*schema.Message, sub string) bool {
	for _, m := range msgs {
		if strings.Contains(m.ReasoningContent, sub) {
			return true
		}
	}
	return false
}

// containsInAny 报告 sub 是否出现在任一消息内容中。
func containsInAny(msgs []*schema.Message, sub string) bool {
	for _, m := range msgs {
		if strings.Contains(m.Content, sub) {
			return true
		}
	}
	return false
}

// TestFoldOlderMessages_TailNeverStartsAtOrphanTool 边界：当「使 tail<=target 的最靠前轮首」
// 恰好落在 assistant(tool_call) 上时，其后的 tool 必须一起保留，绝不从 tool 起头。
func TestFoldOlderMessages_TailNeverStartsAtOrphanTool(t *testing.T) {
	a := &agentImpl{compactTrigger: 2000, compactTarget: 800}
	c := ctxeng.ApproxTokenCounter{}

	bulk := strings.Repeat("很长的历史内容甲乙丙丁戊己庚辛", 120)
	msgs := []*schema.Message{
		schema.SystemMessage("SYS"),
		schema.UserMessage("q " + bulk),
		mkAssistantToolCall("c1", "call1 "+bulk),
		schema.ToolMessage("obs1 "+bulk+" rs_a", "c1"),
		mkAssistantToolCall("c2", "call2"),
		schema.ToolMessage("obs2 短 rs_b", "c2"),
	}
	if totalMessageTokens(msgs, c) < a.compactTrigger {
		t.Fatalf("构造失败：应超 trigger")
	}
	got := a.normalizeContext(context.Background(), msgs)
	assertNoOrphanTool(t, got)
	// 若最近轮头 c2 的 assistant 被保留，其 tool 也必须在（配对）。
	last := got[len(got)-1]
	if last.Role == schema.Tool && last.ToolCallID == "c2" {
		// 找到对应 assistant。
		foundAssistant := false
		for _, m := range got {
			for _, tc := range m.ToolCalls {
				if tc.ID == "c2" {
					foundAssistant = true
				}
			}
		}
		if !foundAssistant {
			t.Fatalf("尾部保留了 c2 的 tool 却丢了其 assistant → 配对破坏")
		}
	}
}
