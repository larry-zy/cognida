//go:build integration
// +build integration

package agent

import (
	"context"
	"os"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentuc "link/internal/service/agent"
	"link/internal/model/common"
	"link/internal/infrastructure/llm/chat"
)

// getAPIKey 获取 OpenAI API key，如果不存在则跳过测试。
func getAPIKey(t *testing.T) string {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY 环境变量未设置，跳过集成测试")
	}
	return apiKey
}

// getBaseURL 获取自定义的 API base URL (可选)。
func getBaseURL() string {
	return os.Getenv("OPENAI_BASE_URL")
}

// createToolModel 创建支持工具调用的模型。
func createToolModel(t *testing.T) model.ToolCallingChatModel {
	apiKey := getAPIKey(t)
	baseURL := getBaseURL()

	config := &chat.ChatConfig{
		Source:    common.ModelSourceRemote,
		APIKey:    apiKey,
		ModelName: "gpt-3.5-turbo",
		Provider:  "openai",
	}

	if baseURL != "" {
		config.BaseURL = baseURL
	}

	toolModel, err := chat.NewToolCallingChatModel(context.Background(), config)
	require.NoError(t, err, "创建模型失败")

	return toolModel
}

// TestRealLLM_ModelCreation 测试模型创建。
func TestRealLLM_ModelCreation(t *testing.T) {
	model := createToolModel(t)
	t.Logf("模型创建成功，类型: %T", model)
	assert.NotNil(t, model)
}

// TestRealLLM_SimpleChat 测试简单的真实 LLM 调用。
func TestRealLLM_SimpleChat(t *testing.T) {
	model := createToolModel(t)

	// 使用 WithToolModel 设置工具调用模型
	agent, err := agentuc.New(nil).
		Name("test-agent").
		Prompt("你是一个乐于助人的助手。请用简洁的语言回答。").
		WithToolModel(model).
		Build(context.Background())

	require.NoError(t, err)
	require.NotNil(t, agent)

	ctx := context.Background()
	resp, err := agent.Chat(ctx, "你好，请用一句话介绍你自己。")

	require.NoError(t, err, "Chat 调用失败")
	assert.NotEmpty(t, resp.Content, "响应内容不应为空")
	t.Logf("LLM 响应: %s", resp.Content)
}

// TestRealLLM_Streaming 测试流式响应。
func TestRealLLM_Streaming(t *testing.T) {
	model := createToolModel(t)

	agent, err := agentuc.New(nil).
		Name("test-agent").
		Prompt("你是一个乐于助人的助手。").
		WithToolModel(model).
		Build(context.Background())

	require.NoError(t, err)

	ctx := context.Background()
	chunkChan, err := agent.Stream(ctx, "数到5")

	require.NoError(t, err, "Stream 调用失败")

	var fullContent string
	chunkCount := 0
	for chunk := range chunkChan {
		chunkCount++
		fullContent += chunk.Content
		if chunk.Done {
			break
		}
	}

	assert.Greater(t, chunkCount, 0, "应该收到至少一个 chunk")
	assert.NotEmpty(t, fullContent, "完整内容不应为空")
	t.Logf("收到 %d 个 chunks, 完整内容: %s", chunkCount, fullContent)
}

// TestRealLLM_SystemPrompt 测试系统提示。
func TestRealLLM_SystemPrompt(t *testing.T) {
	model := createToolModel(t)

	agent, err := agentuc.New(nil).
		Name("translator-agent").
		Prompt("你是一个翻译助手。无论用户输入什么，你只需要翻译成英文，不需要其他解释。只返回翻译结果。").
		WithToolModel(model).
		Build(context.Background())

	require.NoError(t, err)

	ctx := context.Background()
	resp, err := agent.Chat(ctx, "你好世界")

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Content)
	t.Logf("翻译结果: %s", resp.Content)
}

// TestRealLLM_Middleware 测试中间件。
func TestRealLLM_Middleware(t *testing.T) {
	model := createToolModel(t)

	var beforeCalled, afterCalled bool

	agent, err := agentuc.New(nil).
		Name("test-agent").
		Prompt("你是一个简洁的助手。").
		WithToolModel(model).
		Before(func(ctx context.Context, message string) (context.Context, string, error) {
			beforeCalled = true
			t.Logf("Before Hook: 原始消息 = %s", message)
			return ctx, message, nil
		}).
		After(func(ctx context.Context, 	resp *agentuc.Response) error {
			afterCalled = true
			t.Logf("After Hook: 响应长度 = %d", len(resp.Content))
			return nil
		}).
		Build(context.Background())

	require.NoError(t, err)

	ctx := context.Background()
	resp, err := agent.Chat(ctx, "说'测试成功'")

	require.NoError(t, err)
	assert.True(t, beforeCalled, "Before hook 应该被调用")
	assert.True(t, afterCalled, "After hook 应该被调用")
	assert.NotEmpty(t, resp.Content)
}

// TestRealLLM_DifferentPrompts 测试不同的提示词。
func TestRealLLM_DifferentPrompts(t *testing.T) {
	model := createToolModel(t)

	testCases := []struct {
		name    string
		prompt  string
		message string
	}{
		{
			name:    "简洁回答",
			prompt:  "你是一个简洁的助手，所有回答不超过20个字。",
			message: "什么是人工智能？",
		},
		{
			name:    "详细解释",
			prompt:  "你是一个详细的解释者，请详细回答问题。",
			message: "什么是人工智能？",
		},
		{
			name:    "幽默风格",
			prompt:  "你是一个幽默的助手，用有趣的方式回答问题。",
			message: "介绍一下你自己",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			agent, err := agentuc.New(nil).
				Name("test-agent").
				Prompt(tc.prompt).
				WithToolModel(model).
				Build(context.Background())
			require.NoError(t, err)

			ctx := context.Background()
			resp, err := agent.Chat(ctx, tc.message)
			require.NoError(t, err)

			t.Logf("提示词: %s", tc.prompt)
			t.Logf("响应: %s", resp.Content)
			assert.NotEmpty(t, resp.Content)
		})
	}
}
