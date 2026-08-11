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
	"log"
	"time"

	"github.com/google/uuid"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/conversation"
	domain_knowledge "cognida/internal/model/knowledge"
)

// maxSessionDetailMessages 会话详情返回消息的硬上限〔PF-1〕。
// 此前用 Size:10000 全量拉取，长会话（异常/机器人刷屏）会撑爆内存与响应体。
// 取「最近 N 条」有界返回：典型会话（<500 条）行为不变、全量返回且时间正序；
// 超限会话只回最近 500 条（对聊天场景最新上下文最有价值）。后续如需完整历史
// 回溯，应在 handler/前端接 cursor 分页（见 CLAUDE.md cursor 约定），此处仅做兜底封顶。
const maxSessionDetailMessages = 500

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

	// 查询消息列表：有界取最近 maxSessionDetailMessages 条〔PF-1〕。
	// FindRecentBySessionID 已按时间倒序取最近 N 条再反转为正序（最早在前），
	// 与旧 FindBySessionID(created_at ASC) 的展示顺序一致；典型会话（<N 条）
	// 返回全部消息、行为不变，仅对超长会话封顶防无界加载。
	messages, err := s.messageRepo.FindRecentBySessionID(ctx, id, maxSessionDetailMessages)
	if err != nil {
		return nil, fmt.Errorf("查询消息列表失败: %w", err)
	}

	// 统计消息总数，使「最近 N 条」的截断对前端显式可见〔#10〕。
	// 统计失败不阻断详情返回：退化为 Total=len(messages)、HasMore=false（保守认为未截断）。
	total, cerr := s.messageRepo.CountBySessionID(ctx, id)
	if cerr != nil {
		log.Printf("[SessionService] 统计会话 %s 消息总数失败，HasMore 降级为 false: %v", id, cerr)
		total = int64(len(messages))
	}

	// 构建会话响应（包含 RAG 配置）
	return &SessionDetailResponse{
		Session:  session,
		Messages: messages,
		Total:    total,
		HasMore:  total > int64(len(messages)),
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

	// 批量加载 RAG 配置：一次 IN 查询替代逐会话 FindBySessionID，消除 N+1〔PF-3〕
	sessionIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.ID)
	}
	settingsBySession, err := s.retrievalRepo.FindBySessionIDs(ctx, sessionIDs)
	if err != nil {
		// 批量加载失败不阻断列表返回，但一次瞬时批量错误不应抹掉整页所有会话的 RAG 配置〔#7〕。
		// 记录错误并降级为逐会话补偿加载：单个会话失败只影响它自己，其余仍带出配置。
		log.Printf("[SessionService] 批量加载 RAG 配置失败，降级逐会话补偿: %v", err)
		settingsBySession = make(map[string]*domain_knowledge.RetrievalSetting, len(sessionIDs))
		for _, sid := range sessionIDs {
			setting, ferr := s.retrievalRepo.FindBySessionID(ctx, sid)
			if ferr != nil {
				continue
			}
			if setting != nil {
				settingsBySession[sid] = setting
			}
		}
	}

	// 转换为响应格式
	sessionResponses := make([]*SessionResponse, 0, len(sessions))
	for _, session := range sessions {
		resp := s.toSessionResponse(session)
		if setting := settingsBySession[session.ID]; setting != nil {
			resp.RAGConfig = s.convertToRAGConfig(setting)
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
