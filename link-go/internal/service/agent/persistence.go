// Package agent provides unified agent persistence service
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"link/internal/model/conversation"
)

// ========================================
// Agent Persistence Service
// ========================================

// AgentPersistenceService provides unified session/message persistence for all agents
type AgentPersistenceService struct {
	sessionRepo conversation.SessionRepository
	messageRepo conversation.MessageRepository
}

// NewAgentPersistenceService creates a new agent persistence service
func NewAgentPersistenceService(
	sessionRepo conversation.SessionRepository,
	messageRepo conversation.MessageRepository,
) *AgentPersistenceService {
	return &AgentPersistenceService{
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,
	}
}

// ExecuteRequest 执行带持久化的 Agent 请求
type ExecuteRequest struct {
	AgentID   string // Agent 类型标识，如 "text2sql", "rag", "chat"
	SessionID string // 可选，为空时创建新会话
	Query     string // 用户查询
	UserID    int64  // 用户 ID
	TenantID  int64  // 租户 ID
}

// ExecuteResponse Agent 执行响应
type ExecuteResponse struct {
	Content     string                 // 响应内容
	ToolCalls   []ToolCallInfo         // 工具调用（如果有）
	AgentSteps  map[string]interface{} // Agent 执行步骤
	TokenCount  int                    // Token 使用量
}

// ToolCallInfo 工具调用信息
type ToolCallInfo struct {
	ID       string
	Name     string
	Input    string
	Output   string
	Error    string
	Duration int64
}

// ExecuteWithContext 在持久化上下文中执行 Agent 逻辑
func (s *AgentPersistenceService) ExecuteWithContext(
	ctx context.Context,
	req *ExecuteRequest,
	executeFunc func(ctx context.Context) (*ExecuteResponse, error),
) (*ExecuteResponse, error) {
	// 1. 准备阶段：创建或验证 Session
	session, err := s.prepareSession(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("prepare session failed: %w", err)
	}

	// 将 sessionID 注入到 context 中
	ctx = withSessionID(ctx, session.ID)
	ctx = withTenantID(ctx, req.TenantID)
	ctx = withUserID(ctx, req.UserID)
	ctx = withAgentID(ctx, req.AgentID)

	// 2. 保存用户消息
	userMsg := &conversation.Message{
		ID:        generateMessageID(),
		SessionID: session.ID,
		Role:      conversation.RoleUser,
		Content:   req.Query,
	}
	if err := s.messageRepo.Create(ctx, userMsg); err != nil {
		// 记录日志但不中断流程
		fmt.Printf("[Warning] Failed to save user message: %v\n", err)
	}

	// 3. 执行 Agent 逻辑
	resp, err := executeFunc(ctx)
	if err != nil {
		return nil, err
	}

	// 4. 保存助手消息
	assistantMsg := &conversation.Message{
		ID:          generateMessageID(),
		SessionID:    session.ID,
		Role:        conversation.RoleAssistant,
		Content:     resp.Content,
		IsCompleted: true,
		TokenCount:  resp.TokenCount,
	}

	// 序列化工具调用
	if len(resp.ToolCalls) > 0 {
		assistantMsg.ToolCalls = serializeToolCalls(resp.ToolCalls)
	}

	// 序列化 Agent 步骤
	if resp.AgentSteps != nil {
		assistantMsg.AgentSteps = serializeAgentSteps(resp.AgentSteps)
	}

	if err := s.messageRepo.Create(ctx, assistantMsg); err != nil {
		fmt.Printf("[Warning] Failed to save assistant message: %v\n", err)
	}

	// 5. 更新 Session 消息计数
	session.MessageCount += 2
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		fmt.Printf("[Warning] Failed to update session message count: %v\n", err)
	}

	// 6. 将 sessionID 返回给调用者
	resp.AgentSteps = map[string]interface{}{
		"session_id": session.ID,
		"content":    resp.Content,
	}
	if resp.ToolCalls != nil {
		resp.AgentSteps["tool_calls"] = resp.ToolCalls
	}

	return resp, nil
}

// prepareSession 准备 Session（创建新会话或验证现有会话）
func (s *AgentPersistenceService) prepareSession(ctx context.Context, req *ExecuteRequest) (*conversation.Session, error) {
	// 如果提供了 sessionID，验证它是否存在
	if req.SessionID != "" {
		session, err := s.sessionRepo.FindByID(ctx, req.SessionID)
		if err != nil {
			return nil, fmt.Errorf("session not found: %w", err)
		}
		// 验证租户归属
		if session.TenantID != req.TenantID {
			return nil, fmt.Errorf("session does not belong to tenant")
		}
		return session, nil
	}

	// 创建新会话
	now := time.Now()
	// 根据 AgentID 确定 AgentType
	agentType := determineAgentType(req.AgentID)
	session := &conversation.Session{
		ID:        generateSessionID(),
		TenantID:  req.TenantID,
		UserID:    req.UserID,
		AgentType: agentType,
		Title:     generateSessionTitle(req.AgentID, req.Query),
		Status:    conversation.SessionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// determineAgentType 根据 AgentID 确定 AgentType
func determineAgentType(agentID string) string {
	switch {
	case agentID == "agent-text2sql-001" || agentID == "agent-text2sql-per":
		return "text2sql"
	case agentID == "agent-rag-001":
		return "rag"
	default:
		return "default"
	}
}

// ========================================
// Context Keys
// ========================================

type contextKey string

const (
	SessionIDKey contextKey = "session_id"
	TenantIDKey  contextKey = "tenant_id"
	UserIDKey    contextKey = "user_id"
	AgentIDKey   contextKey = "agent_id"
)

// withSessionID injects session ID into context
func withSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, SessionIDKey, sessionID)
}

// withTenantID injects tenant ID into context
func withTenantID(ctx context.Context, tenantID int64) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// withUserID injects user ID into context
func withUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// withAgentID injects agent ID into context
func withAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, AgentIDKey, agentID)
}

// GetSessionID retrieves session ID from context
func GetSessionID(ctx context.Context) string {
	if id, ok := ctx.Value(SessionIDKey).(string); ok {
		return id
	}
	return ""
}

// GetTenantID retrieves tenant ID from context
func GetTenantID(ctx context.Context) int64 {
	if id, ok := ctx.Value(TenantIDKey).(int64); ok {
		return id
	}
	return 0
}

// GetUserID retrieves user ID from context
func GetUserID(ctx context.Context) int64 {
	if id, ok := ctx.Value(UserIDKey).(int64); ok {
		return id
	}
	return 0
}

// GetAgentID retrieves agent ID from context
func GetAgentID(ctx context.Context) string {
	if id, ok := ctx.Value(AgentIDKey).(string); ok {
		return id
	}
	return ""
}

// ========================================
// Helper Functions
// ========================================

// generateSessionID generates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("sess-%s", uuid.New().String()[:8])
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("msg-%s", uuid.New().String()[:8])
}

// generateSessionTitle creates a title for the session
func generateSessionTitle(agentType, query string) string {
	agentName := getAgentDisplayName(agentType)
	query = strings.TrimSpace(query)
	if len(query) > 30 {
		query = string([]rune(query)[:30]) + "..."
	}
	return fmt.Sprintf("[%s] %s", agentName, query)
}

// getAgentDisplayName returns display name for agent type
func getAgentDisplayName(agentType string) string {
	switch agentType {
	case "text2sql", "agent-text2sql-001", "agent-text2sql-per":
		return "SQL查询"
	case "rag", "agent-rag-001":
		return "知识库检索"
	case "chat", "default":
		return "对话"
	default:
		return "AI助手"
	}
}

// serializeToolCalls converts tool calls to JSON string
func serializeToolCalls(calls []ToolCallInfo) string {
	if len(calls) == 0 {
		return ""
	}
	result := "["
	for i, tc := range calls {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf(`{"id":"%s","name":"%s","input":"%s","output":"%s","error":"%s"}`,
			tc.ID, tc.Name, escapeString(tc.Input), escapeString(tc.Output), escapeString(tc.Error))
	}
	result += "]"
	return result
}

// serializeAgentSteps converts agent steps to JSON string.
//
// steps 内含嵌套结构（如 "steps": []map[string]interface{}{...} 的工具调用轨迹），
// 必须用 json.Marshal 才能生成合法 JSON，供前端 parseAgentSteps 回放思考时间线。
func serializeAgentSteps(steps map[string]interface{}) string {
	if len(steps) == 0 {
		return ""
	}
	b, err := json.Marshal(steps)
	if err != nil {
		log.Printf("[AgentPersistence] serializeAgentSteps marshal failed: %v", err)
		return ""
	}
	return string(b)
}

// escapeString escapes special characters in string
func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// ========================================
// Streaming Persistence Helpers
// ========================================

// PrepareSessionRequest session preparation request
type PrepareSessionRequest struct {
	AgentID   string
	SessionID string
	UserID    int64
	TenantID  int64
}

// PrepareSession prepares or validates a session for streaming
func (s *AgentPersistenceService) PrepareSession(ctx context.Context, req *PrepareSessionRequest) (*conversation.Session, error) {
	persistReq := &ExecuteRequest{
		AgentID:   req.AgentID,
		SessionID: req.SessionID,
		UserID:    req.UserID,
		TenantID:  req.TenantID,
	}
	return s.prepareSession(ctx, persistReq)
}

// SaveMessageRequest message save request
type SaveMessageRequest struct {
	MessageID string
	SessionID string
	Content   string
	UserID    int64
}

// SaveUserMessage saves a user message
func (s *AgentPersistenceService) SaveUserMessage(ctx context.Context, req *SaveMessageRequest) error {
	log.Printf("[SaveUserMessage] Saving message ID=%s, SessionID=%s", req.MessageID, req.SessionID)

	msg := &conversation.Message{
		ID:        req.MessageID,
		SessionID: req.SessionID,
		Role:      conversation.RoleUser,
		Content:   req.Content,
	}
	if err := s.messageRepo.Create(ctx, msg); err != nil {
		log.Printf("[SaveUserMessage] ERROR: %v", err)
		return fmt.Errorf("failed to save user message: %w", err)
	}
	log.Printf("[SaveUserMessage] SUCCESS")
	return nil
}

// SaveAssistantMessageRequest assistant message save request
type SaveAssistantMessageRequest struct {
	MessageID   string
	SessionID   string
	Content     string
	ToolCalls   []ToolCallInfo
	AgentSteps  map[string]interface{}
	TokenCount  int
}

// SaveAssistantMessage saves an assistant message and updates session count
func (s *AgentPersistenceService) SaveAssistantMessage(ctx context.Context, req *SaveAssistantMessageRequest) error {
	log.Printf("[SaveAssistantMessage] Saving message ID=%s, SessionID=%s", req.MessageID, req.SessionID)

	// 保存助手消息
	assistantMsg := &conversation.Message{
		ID:          req.MessageID,
		SessionID:    req.SessionID,
		Role:        conversation.RoleAssistant,
		Content:     req.Content,
		IsCompleted: true,
		TokenCount:  req.TokenCount,
	}

	// 序列化工具调用
	if len(req.ToolCalls) > 0 {
		assistantMsg.ToolCalls = serializeToolCalls(req.ToolCalls)
	}

	// 序列化 Agent 步骤
	if req.AgentSteps != nil {
		assistantMsg.AgentSteps = serializeAgentSteps(req.AgentSteps)
	}

	if err := s.messageRepo.Create(ctx, assistantMsg); err != nil {
		log.Printf("[SaveAssistantMessage] ERROR saving message: %v", err)
		return fmt.Errorf("failed to save assistant message: %w", err)
	}
	log.Printf("[SaveAssistantMessage] Message saved successfully")

	// 获取会话并更新消息计数
	session, err := s.sessionRepo.FindByID(ctx, req.SessionID)
	if err != nil {
		log.Printf("[SaveAssistantMessage] ERROR finding session: %v", err)
		return fmt.Errorf("failed to find session: %w", err)
	}

	session.MessageCount += 1
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		log.Printf("[SaveAssistantMessage] ERROR updating session: %v", err)
		return fmt.Errorf("failed to update session message count: %w", err)
	}

	log.Printf("[SaveAssistantMessage] SUCCESS")
	return nil
}
