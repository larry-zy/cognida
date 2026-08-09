package framework

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// capturingToolModel 记录最后一次 WithTools 收到的 ToolInfo 名单，
// 用于断言绑定到模型的工具已按名去重（供应商要求 tools 数组名唯一）。
type capturingToolModel struct {
	bound []*schema.ToolInfo
}

func (m *capturingToolModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	// 直接自然收尾，不触发工具调用，聚焦验证绑定名单。
	return &schema.Message{Role: schema.Assistant, Content: "done"}, nil
}

func (m *capturingToolModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Close()
	return reader, nil
}

func (m *capturingToolModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m.bound = tools
	return m, nil
}

// TestBindTools_DedupByName 验证收口点按工具名去重：即便上游装配混入同名工具，
// 绑定到模型的 tools 数组也不会出现重复名（否则 OpenAI/DeepSeek 返回
// 400 "Tool names must be unique"）。
func TestBindTools_DedupByName(t *testing.T) {
	var sink []string
	dupA := &recordingTool{name: "query", calls: &sink}
	dupB := &recordingTool{name: "query", calls: &sink} // 与 dupA 同名，应被丢弃
	other := &recordingTool{name: "analyze", calls: &sink}

	cm := &capturingToolModel{}
	a := &agentImpl{
		name:      "dedup",
		toolModel: cm,
		prompt:    "p",
		tools:     []tool.BaseTool{dupA, dupB, other},
		maxIter:   3,
	}

	if _, err := a.Chat(context.Background(), "hi"); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	if len(cm.bound) != 2 {
		t.Fatalf("绑定工具数应去重为 2，实际 %d：%v", len(cm.bound), toolNames(cm.bound))
	}
	seen := map[string]int{}
	for _, ti := range cm.bound {
		seen[ti.Name]++
	}
	if seen["query"] != 1 || seen["analyze"] != 1 {
		t.Errorf("去重后应各出现一次，实际 %v", seen)
	}
}

func toolNames(infos []*schema.ToolInfo) []string {
	out := make([]string, 0, len(infos))
	for _, ti := range infos {
		out = append(out, ti.Name)
	}
	return out
}
