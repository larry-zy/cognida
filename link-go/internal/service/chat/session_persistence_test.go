// Package llm provides unit tests for session persistence middleware
package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"link/internal/model/conversation"
	"link/internal/model/llm"
)

// ========================================
// Test Cases
// ========================================

// TestSessionPersistenceMiddleware_BeforeChat_NewSession 测试创建新 Session
func TestSessionPersistenceMiddleware_BeforeChat_NewSession(t *testing.T) {
	// Arrange
	sessionRepo := newMockSessionRepository()
	messageRepo := newMockMessageRepository()

	middleware := NewSessionPersistenceMiddleware(sessionRepo, messageRepo)

	// 设置上下文
	ctx := context.Background()
	ctx = context.WithValue(ctx, TenantIDKey, int64(1))
	ctx = context.WithValue(ctx, UserIDKey, int64(100))

	req := &llm.ChatRequest{
		SessionID: "", // 空，应该创建新 Session
		Messages: []*llm.Message{
			{Role: llm.RoleUser, Content: "Hello, this is a test message"},
		},
	}

	// Act
	err := middleware.BeforeChat(ctx, req)

	// Assert
	if err != nil {
		t.Fatalf("BeforeChat() error = %v", err)
	}

	if req.SessionID == "" {
		t.Error("Expected SessionID to be set")
	}

	// 验证 Session 被创建
	if len(sessionRepo.sessions) == 0 {
		t.Error("Expected session to be created")
	}
}

// TestSessionPersistenceMiddleware_BeforeChat_ExistingSession 测试使用现有 Session
func TestSessionPersistenceMiddleware_BeforeChat_ExistingSession(t *testing.T) {
	// Arrange
	sessionRepo := newMockSessionRepository()
	messageRepo := newMockMessageRepository()

	// 预先创建一个 Session
	existingSession := &conversation.Session{
		ID:        "sess-existing",
		TenantID:  1,
		UserID:    100,
		Title:     "Existing Session",
		Status:    1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sessionRepo.sessions["sess-existing"] = existingSession

	middleware := NewSessionPersistenceMiddleware(sessionRepo, messageRepo)

	ctx := context.Background()
	ctx = context.WithValue(ctx, TenantIDKey, int64(1))

	req := &llm.ChatRequest{
		SessionID: "sess-existing",
		Messages: []*llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	}

	// Act
	err := middleware.BeforeChat(ctx, req)

	// Assert
	if err != nil {
		t.Fatalf("BeforeChat() error = %v", err)
	}

	if req.SessionID != "sess-existing" {
		t.Errorf("Expected SessionID to remain 'sess-existing', got '%s'", req.SessionID)
	}
}

// TestSessionPersistenceMiddleware_BeforeChat_SessionNotFound 测试 Session 不存在
func TestSessionPersistenceMiddleware_BeforeChat_SessionNotFound(t *testing.T) {
	// Arrange
	sessionRepo := newMockSessionRepository()
	messageRepo := newMockMessageRepository()

	middleware := NewSessionPersistenceMiddleware(sessionRepo, messageRepo)

	ctx := context.Background()
	req := &llm.ChatRequest{
		SessionID: "non-existent-session",
		Messages: []*llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	}

	// Act
	err := middleware.BeforeChat(ctx, req)

	// Assert
	if err == nil {
		t.Error("Expected error for non-existent session")
	}
}

// TestSessionPersistenceMiddleware_BeforeChat_TenantMismatch 测试租户不匹配
func TestSessionPersistenceMiddleware_BeforeChat_TenantMismatch(t *testing.T) {
	// Arrange
	sessionRepo := newMockSessionRepository()
	messageRepo := newMockMessageRepository()

	// 创建一个属于租户 2 的 Session
	existingSession := &conversation.Session{
		ID:        "sess-tenant-2",
		TenantID:  2,
		UserID:    100,
		Title:     "Tenant 2 Session",
		Status:    1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sessionRepo.sessions["sess-tenant-2"] = existingSession

	middleware := NewSessionPersistenceMiddleware(sessionRepo, messageRepo)

	ctx := context.Background()
	ctx = context.WithValue(ctx, TenantIDKey, int64(1)) // 租户 1

	req := &llm.ChatRequest{
		SessionID: "sess-tenant-2",
		Messages: []*llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	}

	// Act
	err := middleware.BeforeChat(ctx, req)

	// Assert
	if err == nil {
		t.Error("Expected error for tenant mismatch")
	}
}

// TestSessionPersistenceMiddleware_AfterChat_SaveMessages 测试保存消息
func TestSessionPersistenceMiddleware_AfterChat_SaveMessages(t *testing.T) {
	// Arrange
	sessionRepo := newMockSessionRepository()
	messageRepo := newMockMessageRepository()

	middleware := NewSessionPersistenceMiddleware(sessionRepo, messageRepo)

	ctx := context.Background()

	req := &llm.ChatRequest{
		SessionID: "sess-test",
		Messages: []*llm.Message{
			{Role: llm.RoleUser, Content: "User message"},
			{Role: llm.RoleSystem, Content: "System message"}, // 不应该保存
		},
	}

	resp := &llm.ChatResponse{
		Content:   "Assistant response",
		Usage: &llm.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
		ToolCalls: []*llm.ToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: &llm.FunctionCall{
					Name:      "test_tool",
					Arguments: `{"arg":"value"}`,
				},
			},
		},
	}

	// Act
	err := middleware.AfterChat(ctx, req, resp)

	// Assert
	if err != nil {
		t.Fatalf("AfterChat() error = %v", err)
	}

	// 应该保存 2 条消息：用户消息 + 助手消息
	// 系统消息不应该被保存
	if len(messageRepo.messages) != 2 {
		t.Errorf("Expected 2 messages to be saved, got %d", len(messageRepo.messages))
	}
}

// TestSessionPersistenceMiddleware_AfterChat_NoSession 测试没有 SessionID 时不保存
func TestSessionPersistenceMiddleware_AfterChat_NoSession(t *testing.T) {
	// Arrange
	sessionRepo := newMockSessionRepository()
	messageRepo := newMockMessageRepository()

	middleware := NewSessionPersistenceMiddleware(sessionRepo, messageRepo)

	ctx := context.Background()

	req := &llm.ChatRequest{
		SessionID: "", // 空 SessionID
		Messages: []*llm.Message{
			{Role: llm.RoleUser, Content: "User message"},
		},
	}

	resp := &llm.ChatResponse{
		Content:   "Assistant response",
	}

	// Act
	err := middleware.AfterChat(ctx, req, resp)

	// Assert
	if err != nil {
		t.Fatalf("AfterChat() error = %v", err)
	}

	if len(messageRepo.messages) != 0 {
		t.Errorf("Expected 0 messages when SessionID is empty, got %d", len(messageRepo.messages))
	}
}

// TestSessionPersistenceMiddleware_AfterChat_MessageCreateError 测试消息保存失败不影响响应
func TestSessionPersistenceMiddleware_AfterChat_MessageCreateError(t *testing.T) {
	// Arrange
	sessionRepo := newMockSessionRepository()
	messageRepo := newMockMessageRepository()
	messageRepo.createErr = errors.New("database error")

	middleware := NewSessionPersistenceMiddleware(sessionRepo, messageRepo)

	ctx := context.Background()

	req := &llm.ChatRequest{
		SessionID: "sess-test",
		Messages: []*llm.Message{
			{Role: llm.RoleUser, Content: "User message"},
		},
	}

	resp := &llm.ChatResponse{
		Content:   "Assistant response",
	}

	// Act
	err := middleware.AfterChat(ctx, req, resp)

	// Assert
	// 消息保存失败不应该返回错误
	if err != nil {
		t.Fatalf("AfterChat() should not return error on message save failure, got: %v", err)
	}
}

// TestGenerateSessionID 测试 Session ID 生成
func TestGenerateSessionID(t *testing.T) {
	// Act
	id1 := generateSessionID()
	id2 := generateSessionID()

	// Assert
	if id1 == "" {
		t.Error("Expected non-empty session ID")
	}

	if id2 == "" {
		t.Error("Expected non-empty session ID")
	}

	if id1 == id2 {
		t.Error("Expected different session IDs")
	}

	// 检查格式
	if len(id1) < 6 {
		t.Error("Session ID too short")
	}

	if id1[:5] != "sess-" {
		t.Errorf("Expected session ID to start with 'sess-', got '%s'", id1[:5])
	}
}

// TestGenerateMessageID 测试消息 ID 生成
func TestGenerateMessageID(t *testing.T) {
	// Act
	id1 := generateMessageID()
	id2 := generateMessageID()

	// Assert
	if id1 == "" {
		t.Error("Expected non-empty message ID")
	}

	if id2 == "" {
		t.Error("Expected non-empty message ID")
	}

	if id1 == id2 {
		t.Error("Expected different message IDs")
	}

	// 检查格式
	if len(id1) < 5 {
		t.Error("Message ID too short")
	}

	if id1[:4] != "msg-" {
		t.Errorf("Expected message ID to start with 'msg-', got '%s'", id1[:4])
	}
}

// TestGenerateSessionTitle 测试生成 Session 标题
func TestGenerateSessionTitle(t *testing.T) {
	tests := []struct {
		name     string
		messages []*llm.Message
		expected string
	}{
		{
			name: "user message within limit",
			messages: []*llm.Message{
				{Role: llm.RoleUser, Content: "Short title"},
			},
			expected: "Short title",
		},
		{
			name: "user message exceeds limit - truncated",
			messages: []*llm.Message{
				{Role: llm.RoleUser, Content: "This is a very long message that should be truncated because it exceeds the fifty character limit"},
			},
			expected: "This is a very long ...",
		},
		{
			name: "no user message - use default",
			messages: []*llm.Message{
				{Role: llm.RoleSystem, Content: "System message"},
			},
			expected: "新对话",
		},
		{
			name:     "empty messages - use default",
			messages: []*llm.Message{},
			expected: "新对话",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &llm.ChatRequest{Messages: tt.messages}
			result := generateSessionTitle(req)

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestGetTokenCount 测试获取 token 数量
func TestGetTokenCount(t *testing.T) {
	tests := []struct {
		name     string
		resp     *llm.ChatResponse
		expected int
	}{
		{
			name: "with usage",
			resp: &llm.ChatResponse{
				Usage: &llm.Usage{
					PromptTokens:     10,
					CompletionTokens: 20,
					TotalTokens:      30,
				},
			},
			expected: 30,
		},
		{
			name: "without usage",
			resp: &llm.ChatResponse{
				Usage: nil,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTokenCount(tt.resp)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestGetTenantIDFromContext 测试从上下文获取租户 ID
func TestGetTenantIDFromContext(t *testing.T) {
	// 测试有租户 ID 的上下文
	ctxWithValue := context.WithValue(context.Background(), TenantIDKey, int64(123))
	result := getTenantIDFromContext(ctxWithValue)
	if result != 123 {
		t.Errorf("Expected 123, got %d", result)
	}

	// 测试没有租户 ID 的上下文
	ctxWithoutValue := context.Background()
	result = getTenantIDFromContext(ctxWithoutValue)
	if result != 0 {
		t.Errorf("Expected 0 for missing tenant ID, got %d", result)
	}
}

// TestGetUserID 测试从上下文获取用户 ID
func TestGetUserID(t *testing.T) {
	// 测试有用户 ID 的上下文
	ctxWithValue := context.WithValue(context.Background(), UserIDKey, int64(456))
	result := getUserID(ctxWithValue)
	if result != 456 {
		t.Errorf("Expected 456, got %d", result)
	}

	// 测试没有用户 ID 的上下文
	ctxWithoutValue := context.Background()
	result = getUserID(ctxWithoutValue)
	if result != 0 {
		t.Errorf("Expected 0 for missing user ID, got %d", result)
	}
}

// Benchmark_SessionPersistenceMiddleware_AfterChat 性能测试
func Benchmark_SessionPersistenceMiddleware_AfterChat(b *testing.B) {
	sessionRepo := newMockSessionRepository()
	messageRepo := newMockMessageRepository()

	middleware := NewSessionPersistenceMiddleware(sessionRepo, messageRepo)

	ctx := context.Background()

	req := &llm.ChatRequest{
		SessionID: "sess-test",
		Messages: []*llm.Message{
			{Role: llm.RoleUser, Content: "User message"},
		},
	}

	resp := &llm.ChatResponse{
		Content:   "Assistant response",
		Usage: &llm.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = middleware.AfterChat(ctx, req, resp)
	}
}
