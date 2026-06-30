// Package llm provides session persistence middleware for ChatService
package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"link/internal/model/conversation"
	domainllm "link/internal/model/llm"
)

// ========================================
// Context Keys
// ========================================
// 使用字符串 key 以避免跨包类型不匹配问题
const (
	TenantIDKey = "tenant_id"
	UserIDKey   = "user_id"
)

// ========================================
// Session Persistence Middleware
// ========================================

// SessionPersistenceMiddleware handles automatic session and message persistence
type SessionPersistenceMiddleware struct {
	sessionRepo conversation.SessionRepository
	messageRepo conversation.MessageRepository
}

// NewSessionPersistenceMiddleware creates a new session persistence middleware
func NewSessionPersistenceMiddleware(
	sessionRepo conversation.SessionRepository,
	messageRepo conversation.MessageRepository,
) *SessionPersistenceMiddleware {
	return &SessionPersistenceMiddleware{
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,
	}
}

// BeforeChat creates or validates session before chat execution
func (m *SessionPersistenceMiddleware) BeforeChat(ctx context.Context, req *domainllm.ChatRequest) error {
	// If session ID is provided, validate it exists
	if req.SessionID != "" {
		session, err := m.sessionRepo.FindByID(ctx, req.SessionID)
		if err != nil {
			return fmt.Errorf("session not found: %w", err)
		}

		// Verify tenant ownership
		tenantID := getTenantIDFromContext(ctx)
		if !session.BelongsToTenant(tenantID) {
			return fmt.Errorf("session does not belong to tenant")
		}
		return nil
	}

	// Create new session
	tenantID := getTenantID(ctx)
	userID := getUserID(ctx)

	now := time.Now()
	session := &conversation.Session{
		ID:        generateSessionID(),
		TenantID:  tenantID,
		UserID:    userID,
		Title:     generateSessionTitle(req),
		Status:    conversation.SessionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := m.sessionRepo.Create(ctx, session); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	req.SessionID = session.ID
	return nil
}

// AfterChat saves messages after chat execution
func (m *SessionPersistenceMiddleware) AfterChat(ctx context.Context, req *domainllm.ChatRequest, resp *domainllm.ChatResponse) error {
	if req.SessionID == "" {
		return nil // No session to save to
	}

	// Generate request ID for this interaction
	requestID := generateRequestID()

	// Save user messages
	for _, msg := range req.Messages {
		if msg.Role == domainllm.RoleUser {
			userMsg := &conversation.Message{
				ID:        generateMessageID(),
				RequestID: requestID,
				SessionID: req.SessionID,
				Role:      msg.Role,
				Content:   msg.Content,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			if err := m.messageRepo.Create(ctx, userMsg); err != nil {
				// Log error but don't fail the entire request
				fmt.Printf("[Warning] Failed to save user message: %v\n", err)
			}
		}
	}

	// Save assistant message
	now := time.Now()
	assistantMsg := &conversation.Message{
		ID:          generateMessageID(),
		RequestID:   requestID,
		SessionID:   req.SessionID,
		Role:        domainllm.RoleAssistant,
		Content:     resp.Content,
		IsCompleted: true,
		TokenCount:  getTokenCount(resp),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Serialize tool calls if present
	if len(resp.ToolCalls) > 0 {
		assistantMsg.ToolCalls = serializeToolCalls(resp.ToolCalls)
	}

	if err := m.messageRepo.Create(ctx, assistantMsg); err != nil {
		// Log error but don't fail the entire request
		fmt.Printf("[Warning] Failed to save assistant message: %v\n", err)
	}

	return nil
}

// ========================================
// Helper Functions
// ========================================

// getTenantIDFromContext extracts tenant ID from context using our context key
func getTenantIDFromContext(ctx context.Context) int64 {
	if tenantID, ok := ctx.Value(TenantIDKey).(int64); ok {
		return tenantID
	}
	return 0 // Default tenant
}

// getUserID extracts user ID from context
func getUserID(ctx context.Context) int64 {
	if userID, ok := ctx.Value(UserIDKey).(int64); ok {
		return userID
	}
	return 0 // Default user
}

// generateSessionID generates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("sess-%s", uuid.New().String()[:8])
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("msg-%s", uuid.New().String()[:8])
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("req-%s", uuid.New().String()[:8])
}

// generateSessionTitle creates a title for the session based on the first message
func generateSessionTitle(req *domainllm.ChatRequest) string {
	if len(req.Messages) == 0 {
		return "新对话"
	}

	// Find first user message
	for _, msg := range req.Messages {
		if msg.Role == domainllm.RoleUser && msg.Content != "" {
			content := msg.Content
			// 去除首尾空格
			content = strings.TrimSpace(content)
			if content == "" {
				return "新对话"
			}

			// 对于中文，截取 20 个字符；对于英文，截取 40 个字符
			maxLen := 20
			runes := []rune(content)
			if len(runes) > maxLen {
				return string(runes[:maxLen]) + "..."
			}
			return content
		}
	}

	return "新对话"
}

// getTokenCount extracts token count from response
func getTokenCount(resp *domainllm.ChatResponse) int {
	if resp.Usage != nil {
		return resp.Usage.TotalTokens
	}
	return 0
}

// serializeToolCalls converts ToolCall slice to JSON string
func serializeToolCalls(toolCalls []*domainllm.ToolCall) string {
	if len(toolCalls) == 0 {
		return ""
	}

	// Simple JSON serialization
	result := "["
	for i, tc := range toolCalls {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`{"id":"%s","type":"%s","function":{"name":"%s","arguments":%s}}`,
			tc.ID, tc.Type, tc.Function.Name, tc.Function.Arguments)
	}
	result += "]"
	return result
}
