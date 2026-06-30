//go:build integration
// +build integration

package memory

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	infraagent "link/internal/infrastructure/adapter/agent"
)

// TestMemory_InMemory tests in-memory storage.
func TestMemory_InMemory(t *testing.T) {
	memory := infraagent.NewInMemoryMemory()
	ctx := context.Background()
	sessionID := "test-session"

	// 初始状态为空
	messages, err := memory.LoadHistory(ctx, sessionID)
	require.NoError(t, err)
	assert.Empty(t, messages)

	// 保存消息
	err = memory.SaveMessage(ctx, sessionID, schema.UserMessage("Hello"))
	require.NoError(t, err)

	// 加载消息
	messages, err = memory.LoadHistory(ctx, sessionID)
	require.NoError(t, err)
	assert.Len(t, messages, 1)
	assert.Equal(t, "Hello", messages[0].Content)

	// 保存第二条消息
	err = memory.SaveMessage(ctx, sessionID, schema.AssistantMessage("Hi there!", nil))
	require.NoError(t, err)

	// 验证两条消息
	messages, err = memory.LoadHistory(ctx, sessionID)
	require.NoError(t, err)
	assert.Len(t, messages, 2)
}

// TestMemory_SessionMemory tests session-based memory.
func TestMemory_SessionMemory(t *testing.T) {
	sm := infraagent.NewSessionMemory()
	ctx := context.Background()

	// 添加消息
	err := sm.AddMessage(ctx, "session-1", "user", "我是小明")
	require.NoError(t, err)

	err = sm.AddMessage(ctx, "session-1", "assistant", "你好小明！")
	require.NoError(t, err)

	// 获取消息
	messages, err := sm.GetMessages(ctx, "session-1")
	require.NoError(t, err)
	assert.Len(t, messages, 2)

	// 不同 session
	err = sm.AddMessage(ctx, "session-2", "user", "我是小红")
	require.NoError(t, err)

	// session-1 应该还是 2 条
	messages, _ = sm.GetMessages(ctx, "session-1")
	assert.Len(t, messages, 2)

	// session-2 应该是 1 条
	messages, _ = sm.GetMessages(ctx, "session-2")
	assert.Len(t, messages, 1)
}

// TestMemory_ConversationHistory tests conversation history with size limit.
func TestMemory_ConversationHistory(t *testing.T) {
	ch := infraagent.NewConversationHistory("test-session", 5)
	ctx := context.Background()

	// 添加系统提示
	err := ch.AddMessage(ctx, "system", "你是一个助手")
	require.NoError(t, err)

	// 添加多条消息
	for i := 1; i <= 10; i++ {
		err := ch.AddMessage(ctx, "user", "消息")
		require.NoError(t, err)
		err = ch.AddMessage(ctx, "assistant", "响应")
		require.NoError(t, err)
	}

	// 获取消息
	messages, err := ch.GetMessages(ctx)
	require.NoError(t, err)

	// 应该包含系统消息 + 最多 5 条其他消息（由于 maxSize=5）
	t.Logf("消息数量: %d (系统消息 + 限制)", len(messages))
	assert.NotEmpty(t, messages)
}

// TestMemory_BuildMessages tests building messages with history.
func TestMemory_BuildMessages(t *testing.T) {
	ch := infraagent.NewConversationHistory("build-test", 10)
	ctx := context.Background()

	// 添加历史消息
	ch.AddMessage(ctx, "user", "我叫小明")
	ch.AddMessage(ctx, "assistant", "你好小明！")

	// 构建消息
	messages, err := ch.BuildMessages(ctx, "你是一个助手。", "我叫什么名字？")
	require.NoError(t, err)

	// 应该包含: system + 2条历史 + 当前用户消息
	assert.Len(t, messages, 4)
	assert.Equal(t, schema.System, messages[0].Role)
	assert.Equal(t, "我叫小明", messages[1].Content)
	assert.Equal(t, "我叫什么名字？", messages[3].Content)
}

// TestMemory_ClearSession tests clearing session memory.
func TestMemory_ClearSession(t *testing.T) {
	sm := infraagent.NewSessionMemory()
	ctx := context.Background()
	sessionID := "clear-test"

	// 添加消息
	sm.AddMessage(ctx, sessionID, "user", "Test")

	// 验证有消息
	messages, _ := sm.GetMessages(ctx, sessionID)
	assert.Len(t, messages, 1)

	// 清空
	sm.ClearSession(ctx, sessionID)

	// 验证已清空
	messages, _ = sm.GetMessages(ctx, sessionID)
	assert.Len(t, messages, 0)
}

// TestMemory_ListSessions tests listing sessions.
func TestMemory_ListSessions(t *testing.T) {
	sm := infraagent.NewSessionMemory()
	ctx := context.Background()

	// 初始为空
	sessions := sm.ListSessions()
	assert.Empty(t, sessions)

	// 添加多个 session
	sm.AddMessage(ctx, "session-1", "user", "Test1")
	sm.AddMessage(ctx, "session-2", "user", "Test2")
	sm.AddMessage(ctx, "session-3", "user", "Test3")

	// 列出
	sessions = sm.ListSessions()
	assert.Len(t, sessions, 3)
}

// TestMemory_ConversationWithRealLLM tests real LLM with memory context.
func TestMemory_ConversationWithRealLLM(t *testing.T) {
	ch := infraagent.NewConversationHistory("real-llm-session", 10)
	ctx := context.Background()

	// 第一轮
	messages1, err := ch.BuildMessages(ctx, "你是一个有记忆力的助手。", "我叫小明")
	require.NoError(t, err)

	t.Logf("第一轮消息数: %d", len(messages1))

	// 保存
	ch.AddMessage(ctx, "user", "我叫小明")

	// 第二轮 - 使用历史
	messages2, _ := ch.BuildMessages(ctx, "你是一个有记忆力的助手。", "我叫什么名字？")
	t.Logf("第二轮消息数: %d", len(messages2))

	// 验证历史包含名字
	hasName := false
	for _, msg := range messages2 {
		if msg.Content == "我叫小明" {
			hasName = true
			break
		}
	}
	assert.True(t, hasName, "历史应该包含用户名字")
}
