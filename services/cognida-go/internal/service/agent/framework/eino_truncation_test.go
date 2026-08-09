package framework

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// truncatedMsg 构造一个被输出长度上限截断（finish_reason=length）、只有半句正文、无工具调用的响应，
// 复现 DeepSeek thinking 模型「思维链撑爆输出预算、在发出 tool_call 前被切断」的生产故障。
func truncatedMsg(content string) *schema.Message {
	return &schema.Message{
		Role:         schema.Assistant,
		Content:      content,
		ResponseMeta: &schema.ResponseMeta{FinishReason: "length"},
	}
}

// TestReAct_TruncationRetriesThenRecovers 验证：finish_reason=length 且无工具调用时，不把半句正文
// 当最终答案交付，而是注入精简提示后重试；模型下一轮正常产出工具调用即恢复，自然收尾。
// 这是本次修复的核心——旧行为会把截断的半句话当 naturalFinish 返回，前端空轮询表现为「卡住」。
func TestReAct_TruncationRetriesThenRecovers(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	// 脚本：首轮截断（无工具）→ 应重试；次轮正常产出工具调用 → 执行；末轮无工具正常收尾。
	tm := &scriptedToolModel{script: []*schema.Message{
		truncatedMsg("我先查看可用的受治理指标模型和术语对齐情况，确定取"),
		toolCallMsg("1", "query"),
		{Role: schema.Assistant, Content: "结论"},
	}}

	a := &agentImpl{
		name:      "react",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   10,
	}

	resp, err := a.Chat(context.Background(), "分析趋势")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	// 截断被识别并恢复：最终交付的是真正结论，而非截断的半句话。
	if resp.Content != "结论" {
		t.Errorf("expected recovered conclusion %q, got %q", "结论", resp.Content)
	}
	if len(order) != 1 || order[0] != "query" {
		t.Errorf("expected tool call after truncation retry, got %v", order)
	}
	// 恢复属自然收尾，不应标 terminated_by / partial。
	if _, ok := resp.Metadata["terminated_by"]; ok {
		t.Errorf("recovery is natural finish, must not set terminated_by: %v", resp.Metadata["terminated_by"])
	}
	if resp.Metadata["partial"] == true {
		t.Errorf("recovery is natural finish, must not be partial")
	}
}

// TestReAct_TruncationExhaustedWindsDown 验证：持续截断超过重试上限后，不无限重试、也不把半句话
// 当答案，而是转 wind-down 给出诚实结论，标注 terminated_by=output_truncated、partial=true。
func TestReAct_TruncationExhaustedWindsDown(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	// 脚本：连续 3 次截断（无工具）——重试 2 次后第 3 次仍截断 → 转 wind-down（第 4 次 Generate 走默认收尾）。
	tm := &scriptedToolModel{script: []*schema.Message{
		truncatedMsg("先查看"),
		truncatedMsg("再确认"),
		truncatedMsg("接着取数"),
	}}

	a := &agentImpl{
		name:      "react",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   10,
	}

	resp, err := a.Chat(context.Background(), "持续截断")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Metadata["terminated_by"] != TerminatedByTruncated {
		t.Errorf("expected terminated_by=%s, got %v", TerminatedByTruncated, resp.Metadata["terminated_by"])
	}
	if resp.Metadata["partial"] != true {
		t.Errorf("expected partial=true on truncation exhaustion")
	}
	// 未触发任何工具调用（每轮都在发 tool_call 前被截断），但也没有把半句正文当答案。
	if len(order) != 0 {
		t.Errorf("no tool should run when every turn truncates pre-tool_call, got %v", order)
	}
	// 3 次截断（重试 2 次）+ 1 次 wind-down = 4 次生成，证明重试有界、未失控。
	if tm.calls != 4 {
		t.Errorf("expected 4 Generate calls (3 truncated + 1 wind-down), got %d", tm.calls)
	}
	if resp.Content == "" {
		t.Errorf("wind-down must deliver a non-empty honest conclusion, not a truncated half-sentence")
	}
}
