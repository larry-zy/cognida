// Package chat provides unit tests for ChatService and shared test mocks.
package chat

import (
	"context"
	"errors"
	"testing"

	"link/internal/model/conversation"
	"link/internal/model/llm"
)

// ========================================
// Shared Mock Dependencies
// ========================================
// 注意：mockSessionRepository 与 mockMessageRepository 同时被
// session_persistence_test.go 使用，请勿删除。

// mockLLMClient 模拟 llm.LLMClient
type mockLLMClient struct {
	chatResp     *llm.ChatResponse
	chatErr      error
	streamChunks []*llm.ChatChunk
	streamErr    error
	modelInfo    *llm.ModelInfo
	supportsTool bool
	supportsStrm bool
}

func (m *mockLLMClient) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	return m.chatResp, nil
}

func (m *mockLLMClient) ChatStream(ctx context.Context, req *llm.ChatRequest) (<-chan *llm.ChatChunk, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	ch := make(chan *llm.ChatChunk, len(m.streamChunks))
	go func() {
		defer close(ch)
		for _, chunk := range m.streamChunks {
			ch <- chunk
		}
	}()
	return ch, nil
}

func (m *mockLLMClient) GetModelInfo(ctx context.Context) (*llm.ModelInfo, error) {
	if m.modelInfo == nil {
		return nil, errors.New("no model info")
	}
	return m.modelInfo, nil
}

func (m *mockLLMClient) SupportsTools() bool     { return m.supportsTool }
func (m *mockLLMClient) SupportsStreaming() bool { return m.supportsStrm }

// mockSessionRepository 模拟 conversation.SessionRepository
type mockSessionRepository struct {
	sessions map[string]*conversation.Session
}

func newMockSessionRepository() *mockSessionRepository {
	return &mockSessionRepository{
		sessions: make(map[string]*conversation.Session),
	}
}

func (m *mockSessionRepository) Create(ctx context.Context, session *conversation.Session) error {
	m.sessions[session.ID] = session
	return nil
}

func (m *mockSessionRepository) FindByID(ctx context.Context, id string) (*conversation.Session, error) {
	if sess, ok := m.sessions[id]; ok {
		return sess, nil
	}
	return nil, errors.New("session not found")
}

func (m *mockSessionRepository) FindByUserID(ctx context.Context, userID int64, req *conversation.ListSessionsRequest) ([]*conversation.Session, int64, error) {
	result := make([]*conversation.Session, 0)
	for _, sess := range m.sessions {
		result = append(result, sess)
	}
	return result, int64(len(result)), nil
}

func (m *mockSessionRepository) FindByTenantID(ctx context.Context, tenantID int64, req *conversation.ListSessionsRequest) ([]*conversation.Session, int64, error) {
	result := make([]*conversation.Session, 0)
	for _, sess := range m.sessions {
		result = append(result, sess)
	}
	return result, int64(len(result)), nil
}

func (m *mockSessionRepository) Update(ctx context.Context, session *conversation.Session) error {
	m.sessions[session.ID] = session
	return nil
}

func (m *mockSessionRepository) UpdateStatus(ctx context.Context, id string, status int8) error {
	return nil
}

func (m *mockSessionRepository) UpdateMessageCount(ctx context.Context, id string, count int) error {
	return nil
}

func (m *mockSessionRepository) Delete(ctx context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

func (m *mockSessionRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	return nil
}

func (m *mockSessionRepository) ArchiveSession(ctx context.Context, id string) error {
	return nil
}

func (m *mockSessionRepository) ActivateSession(ctx context.Context, id string) error {
	return nil
}

func (m *mockSessionRepository) Exists(ctx context.Context, id string) (bool, error) {
	_, ok := m.sessions[id]
	return ok, nil
}

func (m *mockSessionRepository) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	return int64(len(m.sessions)), nil
}

// mockMessageRepository 模拟 conversation.MessageRepository
type mockMessageRepository struct {
	messages  []*conversation.Message
	createErr error
}

func newMockMessageRepository() *mockMessageRepository {
	return &mockMessageRepository{
		messages: make([]*conversation.Message, 0),
	}
}

func (m *mockMessageRepository) Create(ctx context.Context, message *conversation.Message) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.messages = append(m.messages, message)
	return nil
}

func (m *mockMessageRepository) CreateBatch(ctx context.Context, messages []*conversation.Message) error {
	m.messages = append(m.messages, messages...)
	return nil
}

func (m *mockMessageRepository) FindByID(ctx context.Context, id string) (*conversation.Message, error) {
	return nil, errors.New("not found")
}

func (m *mockMessageRepository) FindBySessionID(ctx context.Context, sessionID string, req *conversation.ListMessagesRequest) ([]*conversation.Message, int64, error) {
	return m.messages, int64(len(m.messages)), nil
}

func (m *mockMessageRepository) FindRecentBySessionID(ctx context.Context, sessionID string, limit int) ([]*conversation.Message, error) {
	if limit > 0 && len(m.messages) > limit {
		return m.messages[len(m.messages)-limit:], nil
	}
	return m.messages, nil
}

func (m *mockMessageRepository) FindByRequestID(ctx context.Context, requestID string) (*conversation.Message, error) {
	return nil, errors.New("not found")
}

func (m *mockMessageRepository) Update(ctx context.Context, message *conversation.Message) error {
	return nil
}

func (m *mockMessageRepository) UpdateContent(ctx context.Context, id string, content string) error {
	return nil
}

func (m *mockMessageRepository) UpdateCompleted(ctx context.Context, id string, isCompleted bool) error {
	return nil
}

func (m *mockMessageRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockMessageRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	m.messages = make([]*conversation.Message, 0)
	return nil
}

func (m *mockMessageRepository) CountBySessionID(ctx context.Context, sessionID string) (int64, error) {
	return int64(len(m.messages)), nil
}

func (m *mockMessageRepository) GetTokenCountBySessionID(ctx context.Context, sessionID string) (int, error) {
	return 0, nil
}

// ========================================
// ChatService Test Cases
// ========================================

// TestChatService_Chat_Success 测试正常聊天请求转发到底层 LLMClient
func TestChatService_Chat_Success(t *testing.T) {
	client := &mockLLMClient{
		chatResp: &llm.ChatResponse{Content: "Hello from LLM"},
	}
	service := NewChatService(client, nil, nil)

	req := &llm.ChatRequest{
		Messages: []*llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	}

	resp, err := service.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if resp.Content != "Hello from LLM" {
		t.Errorf("Expected content 'Hello from LLM', got '%s'", resp.Content)
	}
}

// TestChatService_Chat_NilClient 测试未初始化模型时返回错误
func TestChatService_Chat_NilClient(t *testing.T) {
	service := NewChatService(nil, nil, nil)

	req := &llm.ChatRequest{
		Messages: []*llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	}

	resp, err := service.Chat(context.Background(), req)
	if err == nil {
		t.Error("Expected error when llmClient is nil, got nil")
	}
	if resp != nil {
		t.Error("Expected nil response when llmClient is nil")
	}
}

// TestChatService_Chat_ClientError 测试底层 LLMClient 返回错误时包装并返回
func TestChatService_Chat_ClientError(t *testing.T) {
	client := &mockLLMClient{chatErr: errors.New("upstream failure")}
	service := NewChatService(client, nil, nil)

	req := &llm.ChatRequest{
		Messages: []*llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	}

	resp, err := service.Chat(context.Background(), req)
	if err == nil {
		t.Error("Expected error from client, got nil")
	}
	if resp != nil {
		t.Error("Expected nil response on client error")
	}
}

// TestChatService_SetChatModel 测试运行时替换模型
func TestChatService_SetChatModel(t *testing.T) {
	service := NewChatService(nil, nil, nil)
	if service.SupportsTools() {
		t.Error("Expected SupportsTools to be false with nil client")
	}

	client := &mockLLMClient{supportsTool: true, supportsStrm: true}
	service.SetChatModel(client)

	if !service.SupportsTools() {
		t.Error("Expected SupportsTools to be true after SetChatModel")
	}
	if !service.SupportsStreaming() {
		t.Error("Expected SupportsStreaming to be true after SetChatModel")
	}
}

// TestChatService_ChatStream 测试流式聊天转发底层分块
func TestChatService_ChatStream(t *testing.T) {
	client := &mockLLMClient{
		streamChunks: []*llm.ChatChunk{
			{Content: "Hello ", Done: false},
			{Content: "world", Done: true},
		},
	}
	service := NewChatService(client, nil, nil)

	req := &llm.ChatRequest{
		Messages: []*llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	}

	stream, err := service.ChatStream(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}

	var content string
	count := 0
	for chunk := range stream {
		content += chunk.Content
		count++
	}

	if count != 2 {
		t.Errorf("Expected 2 chunks, got %d", count)
	}
	if content != "Hello world" {
		t.Errorf("Expected 'Hello world', got '%s'", content)
	}
}

// TestChatService_ChatStream_NilClient 测试流式聊天未初始化模型返回错误
func TestChatService_ChatStream_NilClient(t *testing.T) {
	service := NewChatService(nil, nil, nil)

	req := &llm.ChatRequest{
		Messages: []*llm.Message{{Role: llm.RoleUser, Content: "Hi"}},
	}

	stream, err := service.ChatStream(context.Background(), req)
	if err == nil {
		t.Error("Expected error when llmClient is nil, got nil")
	}
	if stream != nil {
		t.Error("Expected nil stream when llmClient is nil")
	}
}

// TestChatService_GetModelInfo 测试获取模型信息
func TestChatService_GetModelInfo(t *testing.T) {
	client := &mockLLMClient{
		modelInfo: &llm.ModelInfo{Name: "test-model", DisplayName: "Test Model"},
	}
	service := NewChatService(client, nil, nil)

	info, err := service.GetModelInfo(context.Background())
	if err != nil {
		t.Fatalf("GetModelInfo() error = %v", err)
	}
	if info.Name != "test-model" {
		t.Errorf("Expected model name 'test-model', got '%s'", info.Name)
	}
}
