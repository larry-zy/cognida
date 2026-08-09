// Package chat provides chat service implementation.
//
// 读/写路径边界（架构加固 Phase 6，见 agentstate 包门面）：本包（SessionService +
// MessageRepository 落 sessions/messages）是 UI 会话「写路径」；跨轮记忆的「只读投影」
// 读路径在 convcontext.ConversationContextBuilder。二者以 messages 表为单一数据源，
// 写路径落库、读路径回放，互不越界——写入不经读投影、读投影不反向改写会话写入状态。
package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/conversation"
	domain_knowledge "cognida/internal/model/knowledge"
)

// SessionService 会话服务实现
type SessionService struct {
	sessionRepo  conversation.SessionRepository
	messageRepo  conversation.MessageRepository
	retrievalRepo domain_knowledge.RetrievalSettingRepository
}

// NewSessionService 创建会话服务实例
func NewSessionService(
	sessionRepo conversation.SessionRepository,
	messageRepo conversation.MessageRepository,
	retrievalRepo domain_knowledge.RetrievalSettingRepository,
) *SessionService {
	return &SessionService{
		sessionRepo:   sessionRepo,
		messageRepo:   messageRepo,
		retrievalRepo: retrievalRepo,
	}
}

// CreateSession 创建会话（同时保存 RAG 配置）
func (s *SessionService) CreateSession(ctx context.Context, userID int64, req *CreateSessionRequest) (*SessionResponse, error) {
	// 设置默认 agent_type
	agentType := req.AgentType
	if agentType == "" {
		agentType = "default"
	}

	// 生成会话 ID
	now := time.Now()
	sessionID := fmt.Sprintf("sess-%s", uuid.New().String()[:8])

	// 从 ctx 取当前租户，随会话落库（authorizeSession 依赖 tenant 归属校验）
	ctxTenantID, _ := agentctx.GetTenantID(ctx)

	// 构建领域实体
	session := &conversation.Session{
		ID:          sessionID,
		TenantID:    ctxTenantID,
		UserID:      userID,
		AgentType:   agentType,
		Title:       req.Title,
		Description: req.Description,
		Status:      1, // 默认激活状态
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 调用仓储创建会话
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("创建会话失败: %w", err)
	}

	// 如果请求包含 RAG 配置，保存到 retrieval_settings 表
	if req.RAGConfig != nil {
		tenantID := session.TenantID
		if tenantID == 0 {
			if tid, ok := agentctx.GetTenantID(ctx); ok {
				tenantID = tid
			}
		}
		if tenantID > 0 {
			if err := s.retrievalRepo.UpsertBySessionID(ctx, session.ID, tenantID, req.RAGConfig); err != nil {
				// 记录错误但不影响会话创建
				fmt.Printf("⚠️ [CreateSession] 保存 RAG 配置失败: %v\n", err)
			}
		}
	}

	// 构建响应，包含 RAG 配置
	resp := s.toSessionResponse(session)
	if req.RAGConfig != nil {
		resp.RAGConfig = req.RAGConfig
	}
	return resp, nil
}

// authorizeSession 获取会话并校验归属（fail-closed）：
// ctx 必须同时携带有效的 user_id 与 tenant_id，缺失即拒绝，不退化为默认身份；
// 会话必须同时属于该租户与该用户，否则返回“无权访问”错误（防跨租户/跨用户 IDOR）。
func (s *SessionService) authorizeSession(ctx context.Context, id string) (*conversation.Session, error) {
	userID, uok := agentctx.GetUserID(ctx)
	tenantID, tok := agentctx.GetTenantID(ctx)
	if !uok || userID <= 0 || !tok || tenantID <= 0 {
		return nil, fmt.Errorf("无权访问该会话")
	}

	session, err := s.sessionRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !session.BelongsToTenant(tenantID) || !session.IsOwnedBy(userID) {
		return nil, fmt.Errorf("无权访问该会话")
	}
	return session, nil
}

// GetSessionByID 根据ID获取会话（包含 RAG 配置）
func (s *SessionService) GetSessionByID(ctx context.Context, id string) (*SessionResponse, error) {
	session, err := s.authorizeSession(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.buildSessionResponseWithRAG(ctx, session)
}

// GetSessionDetail 获取会话详情（包含消息和 RAG 配置）
func (s *SessionService) GetSessionDetail(ctx context.Context, id string) (*SessionDetailResponse, error) {
	session, err := s.authorizeSession(ctx, id)
	if err != nil {
		return nil, err
	}

	// 查询消息列表（获取所有消息，不分页）
	msgReq := &conversation.ListMessagesRequest{
		SessionID: id,
		Page:      1,
		Size:      10000,
	}
	messages, _, err := s.messageRepo.FindBySessionID(ctx, id, msgReq)
	if err != nil {
		return nil, fmt.Errorf("查询消息列表失败: %w", err)
	}

	// 构建会话响应（包含 RAG 配置）
	return &SessionDetailResponse{
		Session:  session,
		Messages: messages,
	}, nil
}

// ListSessions 查询会话列表（包含 RAG 配置）
func (s *SessionService) ListSessions(ctx context.Context, req *ListSessionsRequest) (*SessionListResponse, error) {
	// 设置默认分页参数
	page := req.Page
	if page == 0 {
		page = 1
	}
	size := req.Size
	if size == 0 {
		size = 20
	}

	// 从上下文获取用户ID
	userID, ok := agentctx.GetUserID(ctx)
	if !ok || userID == 0 {
		return nil, fmt.Errorf("未找到用户ID")
	}

	// 构建查询请求
	listReq := &conversation.ListSessionsRequest{
		Page:   page,
		Size:   size,
		Status: req.Status,
	}

	sessions, total, err := s.sessionRepo.FindByUserID(ctx, userID, listReq)
	if err != nil {
		return nil, fmt.Errorf("查询会话列表失败: %w", err)
	}

	// 转换为响应格式（批量加载 RAG 配置）
	sessionResponses := make([]*SessionResponse, 0, len(sessions))
	for _, session := range sessions {
		resp, err := s.buildSessionResponseWithRAG(ctx, session)
		if err != nil {
			// 如果加载 RAG 配置失败，仍然返回会话信息
			resp = s.toSessionResponse(session)
		}
		sessionResponses = append(sessionResponses, resp)
	}

	return &SessionListResponse{
		Page:     page,
		Size:     size,
		Total:    total,
		Sessions: sessionResponses,
	}, nil
}

// UpdateSession 更新会话（同时更新 RAG 配置）
func (s *SessionService) UpdateSession(ctx context.Context, id string, req *UpdateSessionRequest) (*SessionResponse, error) {
	// 获取现有会话并校验归属
	session, err := s.authorizeSession(ctx, id)
	if err != nil {
		return nil, err
	}

	// 更新字段
	if req.Title != nil {
		session.Title = *req.Title
	}
	if req.Description != nil {
		session.Description = *req.Description
	}
	if req.Status != nil {
		session.Status = *req.Status
	}

	// 更新会话基本信息
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		return nil, fmt.Errorf("更新会话失败: %w", err)
	}

	// 如果请求包含 RAG 配置更新，更新到 retrieval_settings 表
	if req.RAGConfig != nil && session.TenantID > 0 {
		if err := s.retrievalRepo.UpsertBySessionID(ctx, id, session.TenantID, req.RAGConfig); err != nil {
			fmt.Printf("⚠️ [UpdateSession] 更新 RAG 配置失败: %v\n", err)
		}
	}

	// 重新获取更新后的会话
	session, err = s.sessionRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 构建响应，包含 RAG 配置
	return s.buildSessionResponseWithRAG(ctx, session)
}

// DeleteSession 删除会话
func (s *SessionService) DeleteSession(ctx context.Context, id string) error {
	if _, err := s.authorizeSession(ctx, id); err != nil {
		return err
	}
	if err := s.sessionRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("删除会话失败: %w", err)
	}
	return nil
}

// ArchiveSession 归档会话
func (s *SessionService) ArchiveSession(ctx context.Context, id string) error {
	if _, err := s.authorizeSession(ctx, id); err != nil {
		return err
	}
	if err := s.sessionRepo.UpdateStatus(ctx, id, 0); err != nil {
		return fmt.Errorf("归档会话失败: %w", err)
	}
	return nil
}

// ActivateSession 激活会话
func (s *SessionService) ActivateSession(ctx context.Context, id string) error {
	if _, err := s.authorizeSession(ctx, id); err != nil {
		return err
	}
	if err := s.sessionRepo.UpdateStatus(ctx, id, 1); err != nil {
		return fmt.Errorf("激活会话失败: %w", err)
	}
	return nil
}

// buildSessionResponseWithRAG 构建包含 RAG 配置的会话响应
func (s *SessionService) buildSessionResponseWithRAG(ctx context.Context, session *conversation.Session) (*SessionResponse, error) {
	resp := s.toSessionResponse(session)

	// 尝试从 retrieval_settings 表加载 RAG 配置
	retrievalSetting, err := s.retrievalRepo.FindBySessionID(ctx, session.ID)
	if err == nil && retrievalSetting != nil {
		// 将 RetrievalSetting 转换为 RAGConfig
		resp.RAGConfig = s.convertToRAGConfig(retrievalSetting)
	}
	// 如果没有找到 RAG 配置，保持为 nil（前端使用默认配置）

	return resp, nil
}

// convertToRAGConfig 将 RetrievalSetting 转换为 RAGConfig
func (s *SessionService) convertToRAGConfig(setting *domain_knowledge.RetrievalSetting) *RAGConfig {
	config := &RAGConfig{
		Enabled:             false,
		KnowledgeBaseID:     "",
		RetrievalModes:      []string{"vector"},
		VectorTopK:          15,
		KeywordTopK:         15,
		GraphTopK:           10,
		SimilarityThreshold: 0.5,
		Alpha:               0.7,
	}

	if setting.VectorTopK != nil {
		config.VectorTopK = *setting.VectorTopK
	}
	if setting.VectorThreshold != nil {
		config.SimilarityThreshold = *setting.VectorThreshold
	}
	if setting.BM25Enable != nil && *setting.BM25Enable {
		config.RetrievalModes = append(config.RetrievalModes, "keyword")
	}
	if setting.GraphEnabled != nil && *setting.GraphEnabled {
		config.RetrievalModes = append(config.RetrievalModes, "graph")
	}

	return config
}

// toSessionResponse 转换为会话响应格式
func (s *SessionService) toSessionResponse(session *conversation.Session) *SessionResponse {
	return &SessionResponse{
		ID:           session.ID,
		TenantID:     session.TenantID,
		UserID:       session.UserID,
		AgentType:    session.AgentType,
		Title:        session.Title,
		Description:  session.Description,
		Status:       session.Status,
		MessageCount: session.MessageCount,
		CreatedAt:    session.CreatedAt,
		UpdatedAt:    session.UpdatedAt,
	}
}
