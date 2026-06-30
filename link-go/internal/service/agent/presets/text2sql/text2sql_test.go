// Package text2sql Plan-Execute-Reflect 模式 Text2SQL Agent 测试
package text2sql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentuc "link/internal/service/agent/framework"
)

// TestPrompts 测试 Prompt 定义
func TestPrompts(t *testing.T) {
	// 验证 Prompt 不为空
	assert.NotEmpty(t, planPrompt, "planPrompt 不应为空")
	assert.NotEmpty(t, executePrompt, "executePrompt 不应为空")
	assert.NotEmpty(t, reflectPrompt, "reflectPrompt 不应为空")

	// 验证 Prompt 包含关键内容
	assert.Contains(t, planPrompt, "查询规划", "planPrompt 应该包含角色定义")
	assert.Contains(t, executePrompt, "SQL", "executePrompt 应该包含 SQL")
	assert.Contains(t, reflectPrompt, "审核", "reflectPrompt 应该包含审核任务")

	// 验证 planPrompt 包含输出格式说明
	assert.Contains(t, planPrompt, "query_type", "planPrompt 应该说明输出格式")
	assert.Contains(t, planPrompt, "tables", "planPrompt 应该说明输出格式")

	// 验证 executePrompt 包含执行流程说明
	assert.Contains(t, executePrompt, "get_schema", "executePrompt 应该说明使用 get_schema 工具")
	assert.Contains(t, executePrompt, "sql_execute", "executePrompt 应该说明使用 sql_execute 工具")

	// 验证 reflectPrompt 包含状态判断
	assert.Contains(t, reflectPrompt, "success", "reflectPrompt 应该说明成功状态")
	assert.Contains(t, reflectPrompt, "need_retry", "reflectPrompt 应该说明重试状态")
	assert.Contains(t, reflectPrompt, "检查查询结果", "reflectPrompt 应该说明审核任务")
}

// TestGetAgent 测试 Agent 实例获取
func TestGetAgent(t *testing.T) {
	// 重置实例
	SetAgent(nil)

	agent := GetAgent()
	assert.Nil(t, agent, "未设置时应该返回 nil")

	// 设置后获取
	mockAgent := &mockAgent{}
	err := SetAgent(mockAgent)
	require.NoError(t, err)

	retrieved := GetAgent()
	assert.Equal(t, mockAgent, retrieved, "应该返回设置的 Agent")
}

// TestRegisterText2SQLAgent_WithoutModel 测试注册逻辑（需要真实模型）
func TestRegisterText2SQLAgent_WithoutModel(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要真实模型的集成测试")
	}
	// 这个测试需要真实的 ToolCallingChatModel
	// 实际测试在 e2e_test.go 中进行
	t.Skip("需要真实的 ChatModel，由 e2e 测试覆盖")
}

// ========================================
// Mock 类型
// ========================================

type mockAgent struct{}

func (m *mockAgent) Chat(ctx context.Context, message string) (*agentuc.Response, error) {
	return &agentuc.Response{
		Content: "mock response",
	}, nil
}

func (m *mockAgent) Stream(ctx context.Context, message string) (<-chan *agentuc.Chunk, error) {
	ch := make(chan *agentuc.Chunk)
	close(ch)
	return ch, nil
}

func (m *mockAgent) Name() string {
	return "mock"
}
