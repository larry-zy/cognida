//go:build integration
// +build integration

package orchestration

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"link/internal/service/agent/framework"
	"link/internal/model/common"
	"link/internal/infrastructure/llm/chat"
)

// getAPIKey 获取 API key。
func getAPIKey(t *testing.T) string {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY 环境变量未设置，跳过集成测试")
	}
	return apiKey
}

// createRealAgent 创建真实的 Agent。
func createRealAgent(t *testing.T, name, prompt string) framework.Agent {
	apiKey := getAPIKey(t)
	baseURL := os.Getenv("OPENAI_BASE_URL")

	config := &chat.ChatConfig{
		Source:    common.ModelSourceRemote,
		APIKey:    apiKey,
		ModelName: "gpt-3.5-turbo",
		Provider:  "openai",
	}

	if baseURL != "" {
		config.BaseURL = baseURL
	}

	model, err := chat.NewToolCallingChatModel(context.Background(), config)
	require.NoError(t, err)

	// 使用 WithToolModel
	a, err := agent.New(nil).
		Name(name).
		Prompt(prompt).
		WithToolModel(model).
		Build(context.Background())
	require.NoError(t, err)

	return a
}

// TestIntegration_AgentCreation 测试 Agent 创建。
func TestIntegration_AgentCreation(t *testing.T) {
	a := createRealAgent(t, "test", "你是一个助手。")
	assert.NotNil(t, a)
	t.Logf("Agent 创建成功: %s", a.Name())
}

// TestIntegration_SimpleChat 测试简单聊天。
func TestIntegration_SimpleChat(t *testing.T) {
	a := createRealAgent(t, "test", "你是一个简洁的助手。")

	ctx := context.Background()
	resp, err := a.Chat(ctx, "你好")

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Content)
	t.Logf("响应: %s", resp.Content)
}

// TestIntegration_Sequential 测试真实的串行编排。
func TestIntegration_Sequential(t *testing.T) {
	// 创建两个专业化的 agent
	writer := createRealAgent(t, "writer", "你是一个作家，负责创作内容。请用一句话回答。")
	critic := createRealAgent(t, "critic", "你是一个评论家，负责点评和改进建议。请用一句话评论。")

	sequentialAgent := Sequential(writer, critic)

	ctx := context.Background()
	resp, err := sequentialAgent.Chat(ctx, "写一个关于春天的简短句子")

	require.NoError(t, err)
	t.Logf("Sequential 响应:\n%s", resp.Content)
	assert.NotEmpty(t, resp.Content)
}

// TestIntegration_Parallel 测试真实的并行编排。
func TestIntegration_Parallel(t *testing.T) {
	// 创建多个不同视角的 agent
	optimist := createRealAgent(t, "optimist", "你是一个乐观主义者，总是看到积极的一面。请用一句话回答。")
	pessimist := createRealAgent(t, "pessimist", "你是一个悲观主义者，总是看到风险和问题。请用一句话回答。")

	parallelAgent := Parallel(optimist, pessimist)

	ctx := context.Background()
	resp, err := parallelAgent.Chat(ctx, "对人工智能的未来有什么看法？")

	require.NoError(t, err)
	t.Logf("Parallel 响应:\n%s", resp.Content)
	assert.NotEmpty(t, resp.Content)
}

// TestIntegration_Conditional 测试真实的条件路由。
func TestIntegration_Conditional(t *testing.T) {
	helper := createRealAgent(t, "helper", "你是一个通用助手，简洁回答问题。")
	joker := createRealAgent(t, "joker", "你是一个幽默的助手，用幽默的方式回答。")

	// 根据消息内容路由
	predicate := func(message string) bool {
		return len(message) < 20 // 短消息走幽默助手
	}

	conditionalAgent := Conditional(predicate, joker, helper)

	ctx := context.Background()

	// 短消息
	resp1, err := conditionalAgent.Chat(ctx, "讲个笑话")
	require.NoError(t, err)
	t.Logf("短消息响应: %s", resp1.Content)

	// 长消息
	resp2, err := conditionalAgent.Chat(ctx, "请详细解释什么是机器学习")
	require.NoError(t, err)
	t.Logf("长消息响应: %s", resp2.Content)
}

// TestIntegration_Loop 测试循环模式。
func TestIntegration_Loop(t *testing.T) {
	expander := createRealAgent(t, "expander", "你是一个扩展者，将简短的内容扩展成更详细的描述。每次扩展一点，用一句话回答。")

	// 最多迭代 3 次，或者当内容足够长时停止
	loopAgent := Loop(expander, func(resp *framework.Response) bool {
		return len(resp.Content) < 100 // 继续扩展直到内容足够长
	})

	ctx := context.Background()
	resp, err := loopAgent.Chat(ctx, "春天")

	require.NoError(t, err)
	t.Logf("Loop 响应 (迭代次数: %v):\n%s", resp.Metadata["iterations"], resp.Content)
}

// TestIntegration_Func 测试函数包装器。
func TestIntegration_Func(t *testing.T) {
	callCount := 0

	realAgent := createRealAgent(t, "real", "你是一个助手，简洁回答。")

	// 用 Func 包装，添加计数功能
	funcAgent := Func(func(ctx context.Context, message string) (*framework.Response, error) {
		callCount++
		resp, err := realAgent.Chat(ctx, message)
		if resp != nil && resp.Metadata == nil {
			resp.Metadata = make(map[string]interface{})
		}
		if resp != nil {
			resp.Metadata["call_count"] = callCount
		}
		return resp, err
	})

	ctx := context.Background()

	// 第一次调用
	resp1, err := funcAgent.Chat(ctx, "你好")
	require.NoError(t, err)
	t.Logf("第一次调用 (计数: %v): %s", resp1.Metadata["call_count"], resp1.Content)
}
