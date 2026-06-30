// Package handler 提供 Agent 相关的 HTTP 处理器
package handler

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	agentuc "link/internal/service/agent"
	ragtool "link/internal/service/agent/tools"
	"link/internal/handler/sse"
)

// ========================================
// Agent Handler
// ========================================

// AgentHandler Agent 处理器
type AgentHandler struct {
	executeUseCase       *agentuc.ExecuteService
	researchUseCase      *agentuc.ResearchService
	configUseCase         *agentuc.ConfigService
	progressUseCase       *agentuc.ProgressService
	persistenceService   *agentuc.AgentPersistenceService
}

// NewAgentHandler 创建 Agent Handler
func NewAgentHandler(
	executeUseCase *agentuc.ExecuteService,
	researchUseCase *agentuc.ResearchService,
	configUseCase *agentuc.ConfigService,
	progressUseCase *agentuc.ProgressService,
	persistenceService *agentuc.AgentPersistenceService,
) *AgentHandler {
	return &AgentHandler{
		executeUseCase:     executeUseCase,
		researchUseCase:    researchUseCase,
		configUseCase:      configUseCase,
		progressUseCase:    progressUseCase,
		persistenceService: persistenceService,
	}
}

// ========================================
// AgenticRAG 聊天接口
// ========================================

// Chat AgenticRAG 聊天请求
func (h *AgentHandler) Chat(c *gin.Context) {
	var req agentuc.AgenticRAGRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// 从上下文获取用户ID
	userID := GetUserID(c)
	log.Printf("[AgentChat] UserID: %d, Query: %s", userID, req.Query)

	// 执行 AgenticRAG 聊天
	resp, err := h.executeUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		log.Printf("[AgentChat] Error 执行失败: %v", err)
		InternalError(c, err.Error())
		return
	}

	OK(c, resp)
}

// ChatStream AgenticRAG 流式聊天
func (h *AgentHandler) ChatStream(c *gin.Context) {
	log.Printf("[AgentChatStream] Received stream request")

	var req agentuc.AgenticRAGRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[AgentChatStream] Invalid request: %v", err)
		BadRequest(c, err.Error())
		return
	}

	// 从上下文获取用户ID
	userID := GetUserID(c)
	log.Printf("[AgentChatStream] UserID: %d, Query: %s", userID, req.Query)

	// 执行流式聊天
	chunkChan, err := h.executeUseCase.ExecuteStream(c.Request.Context(), &req)
	if err != nil {
		log.Printf("[AgentChatStream] Error 执行失败: %v", err)
		InternalError(c, err.Error())
		return
	}

	// 设置 SSE 响应头
	sse.SetSSEHeaders(c.Writer)

	// 启动心跳机制（30秒间隔）
	stopHeartbeat := sse.StartHeartbeat(c.Request.Context(), c.Writer, nil)
	defer stopHeartbeat()

	// 发送流式数据
	for chunk := range chunkChan {
		if chunk.Done {
			sse.SendSSE(c.Writer, sse.EventTypeDone, chunk)
			break
		}
		if chunk.Content != "" {
			sse.SendSSE(c.Writer, sse.EventTypeContent, chunk)
		}
		if chunk.Metadata != nil {
			sse.SendSSE(c.Writer, sse.EventTypeMetadata, chunk.Metadata)
		}
	}
}

// GetTools 获取可用工具列表
func (h *AgentHandler) GetTools(c *gin.Context) {
	tools, err := h.executeUseCase.GetTools(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, tools)
}

// GetStatus 获取 Agent 状态
func (h *AgentHandler) GetStatus(c *gin.Context) {
	status := h.executeUseCase.GetStatus(c.Request.Context())
	OK(c, gin.H{
		"status": string(status),
	})
}

// ========================================
// Deep Research 接口
// ========================================

// DeepResearch 深度研究
func (h *AgentHandler) DeepResearch(c *gin.Context) {
	var req agentuc.DeepResearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	log.Printf("[DeepResearch] Query: %s", req.Query)

	// 执行深度研究
	resp, err := h.researchUseCase.Execute(c.Request.Context(), &req)
	if err != nil {
		log.Printf("[DeepResearch] Execution failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	OK(c, resp)
}

// DeepResearchStream 深度研究（流式）
func (h *AgentHandler) DeepResearchStream(c *gin.Context) {
	log.Printf("[DeepResearchStream] Received stream request")

	var req agentuc.DeepResearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DeepResearchStream] Invalid request: %v", err)
		BadRequest(c, err.Error())
		return
	}

	log.Printf("[DeepResearchStream] Query: %s", req.Query)

	// 执行流式深度研究
	progressChan, err := h.researchUseCase.ExecuteStreamWithProgress(c.Request.Context(), &req)
	if err != nil {
		log.Printf("[DeepResearchStream] Execution failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	// 设置 SSE 响应头
	sse.SetSSEHeaders(c.Writer)

	// 启动心跳机制
	stopHeartbeat := sse.StartHeartbeat(c.Request.Context(), c.Writer, nil)
	defer stopHeartbeat()

	// 发送流式进度数据
	for progress := range progressChan {
		eventData := h.progressUseCase.ConvertToSSEEvent(progress)
		sse.SendSSE(c.Writer, progress.Stage, eventData)
	}
}

// ========================================
// 配置接口
// ========================================

// GetConfig 获取 Agent 配置
func (h *AgentHandler) GetConfig(c *gin.Context) {
	OK(c, gin.H{
		"agentic_rag":    h.configUseCase.GetAgenticRAGConfig(),
		"deep_research": h.configUseCase.GetDeepResearchConfig(),
	})
}

// UpdateConfig 更新 Agent 配置
func (h *AgentHandler) UpdateConfig(c *gin.Context) {
	var req struct {
		AgenticRAG    map[string]interface{} `json:"agentic_rag,omitempty"`
		DeepResearch map[string]interface{} `json:"deep_research,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// 更新配置（这里简化处理，实际需要更复杂的转换）
	if req.AgenticRAG != nil {
		log.Printf("[UpdateConfig] AgenticRAG config update: %v", req.AgenticRAG)
	}
	if req.DeepResearch != nil {
		log.Printf("[UpdateConfig] DeepResearch config update: %v", req.DeepResearch)
	}

	log.Printf("[UpdateConfig] Config update requested")
	OK(c, gin.H{"message": "配置更新请求已接收", "note": "完整配置更新功能待实现"})
}

// ========================================
// 进度接口
// ========================================

// GetProgress 获取 Agent 执行进度
func (h *AgentHandler) GetProgress(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		BadRequest(c, "会话ID不能为空")
		return
	}

	// 从进度服务获取进度信息
	progress, err := h.progressUseCase.GetProgress(c.Request.Context(), sessionID)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	OK(c, progress)
}

// CalculateSimilarity 计算文本相似度
func (h *AgentHandler) CalculateSimilarity(c *gin.Context) {
	var req agentuc.SimilarityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// 计算相似度
	result, err := h.progressUseCase.CalculateSimilarity(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ========================================
// Text2SQL 接口
// ========================================

// Text2SQLStream Text2SQL 流式查询（带持久化）
func (h *AgentHandler) Text2SQLStream(c *gin.Context) {
	log.Printf("[Text2SQLStream] Received stream request, persistenceService is nil: %v", h.persistenceService == nil)
	if h.persistenceService == nil {
		log.Printf("[Text2SQLStream] ERROR: persistenceService is nil, messages will NOT be persisted!")
	}

	var req agentuc.AgenticRAGRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[Text2SQLStream] Invalid request: %v", err)
		BadRequest(c, err.Error())
		return
	}

	// 设置 agent_id 为 text2sql
	req.AgentID = "agent-text2sql-001"

	// 从上下文获取用户ID和租户ID
	userID := GetUserID(c)
	tenantID := GetTenantID(c)
	log.Printf("[Text2SQLStream] UserID: %d, TenantID: %d, Query: %s", userID, tenantID, req.Query)

	// 使用持久化服务处理会话和消息
	prepareReq := &agentuc.PrepareSessionRequest{
		AgentID:   "agent-text2sql-per",
		SessionID: req.SessionID,
		UserID:    userID,
		TenantID:  tenantID,
	}

	// 生成消息 ID
	userMsgID := fmt.Sprintf("msg-%s", uuid.New().String()[:8])

	// 执行持久化逻辑：准备会话、保存用户消息
	session, err := h.persistenceService.PrepareSession(c.Request.Context(), prepareReq)
	if err != nil {
		log.Printf("[Text2SQLStream] Prepare session failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	// 保存用户消息
	log.Printf("[Text2SQLStream] Saving user message for session %s", session.ID)
	if err := h.persistenceService.SaveUserMessage(c.Request.Context(), &agentuc.SaveMessageRequest{
		MessageID: userMsgID,
		SessionID: session.ID,
		Content:   req.Query,
		UserID:    userID,
	}); err != nil {
		log.Printf("[Text2SQLStream] Save user message failed: %v", err)
		// 继续执行，不中断流程
	} else {
		log.Printf("[Text2SQLStream] Successfully saved user message")
	}

	// 设置 SSE 响应头
	sse.SetSSEHeaders(c.Writer)

	// 启动心跳机制（30秒间隔）
	stopHeartbeat := sse.StartHeartbeat(c.Request.Context(), c.Writer, nil)
	defer stopHeartbeat()

	// 收集完整响应用于持久化
	var fullContent strings.Builder
	var toolCalls []agentuc.ToolCallInfo

	// 更新请求的 session_id 为准备好的会话 ID
	req.SessionID = session.ID

	// 执行流式聊天
	chunkChan, err := h.executeUseCase.ExecuteStream(c.Request.Context(), &req)
	if err != nil {
		log.Printf("[Text2SQLStream] Execution failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	// 发送流式数据并收集内容
	for chunk := range chunkChan {
		if chunk.Done {
			// 发送完成事件，包含完整答案
			sse.SendSSE(c.Writer, "done", gin.H{
				"event":  "done",
				"answer": fullContent.String(),
			})
			break
		}
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
			// 将内容转换为 step 事件（前端期待的格式）
			sse.SendSSE(c.Writer, "step", gin.H{
				"event":   "step",
				"type":    "thought",
				"content": chunk.Content,
			})
		}
		if chunk.Metadata != nil {
			// 检查是否有工具调用事件
			if event, ok := chunk.Metadata["event"].(string); ok {
				switch event {
				case "tool_call":
					// 工具调用开始
					if tc, ok := chunk.Metadata["tool_call"].(interface{}); ok {
						sse.SendSSE(c.Writer, "step", gin.H{
							"event":      "step",
							"type":       "tool_result",
							"tool_name":  chunk.Metadata["tool_name"],
							"tool_params": tc,
							"content":    "正在调用工具...",
						})
					}
				case "tool_result":
					// 工具执行结果
					if tc, ok := chunk.Metadata["tool_call"].(interface{}); ok {
						// 收集工具调用信息（简化处理，跳过复杂的类型转换）
						_ = tc // 工具调用信息已在 Agent 层处理
						sse.SendSSE(c.Writer, "step", gin.H{
							"event":       "step",
							"type":        "tool_result",
							"tool_name":   chunk.Metadata["tool_name"],
							"tool_output": tc,
						})
					}
				default:
					sse.SendSSE(c.Writer, sse.EventTypeMetadata, chunk.Metadata)
				}
			} else {
				// 收集工具调用信息（旧格式兼容）
				if tc, ok := chunk.Metadata["tool_call"].([]agentuc.ToolCallInfo); ok {
					toolCalls = append(toolCalls, tc...)
				}
				sse.SendSSE(c.Writer, sse.EventTypeMetadata, chunk.Metadata)
			}
		}
	}

	// 持久化：保存助手消息
	if h.persistenceService != nil && fullContent.Len() > 0 {
		assistantMsgID := fmt.Sprintf("msg-%s", uuid.New().String()[:8])
		// 捕获需要的变量
		sessionID := session.ID
		content := fullContent.String()
		go func() {
			// 使用新的 context，避免请求 context 被取消
			ctx := context.Background()
			saveReq := &agentuc.SaveAssistantMessageRequest{
				MessageID:  assistantMsgID,
				SessionID:  sessionID,
				Content:    content,
				ToolCalls:  toolCalls,
				AgentSteps: map[string]interface{}{
					"session_id": sessionID,
				},
				TokenCount: 0,
			}
			log.Printf("[Text2SQLStream] Saving assistant message for session %s", sessionID)
			if err := h.persistenceService.SaveAssistantMessage(ctx, saveReq); err != nil {
				log.Printf("[Text2SQLStream] Failed to save assistant message: %v", err)
			} else {
				log.Printf("[Text2SQLStream] Successfully saved assistant message")
			}
		}()
	}
}

// GetDatabaseSchema 获取数据库 schema
// 返回表结构、字段、类型、主键等信息，供前端展示及 Text2SQL 查询参考。
//
// Query 参数：
//   - database_id: 数据库ID（可选，默认当前数据库）
//   - table_name:  表名（可选，不指定则返回所有表）
func (h *AgentHandler) GetDatabaseSchema(c *gin.Context) {
	databaseID := c.Query("database_id")
	tableName := c.Query("table_name")

	log.Printf("[GetDatabaseSchema] database_id=%q table_name=%q", databaseID, tableName)

	result, err := ragtool.FetchSchema(c.Request.Context(), databaseID, tableName)
	if err != nil {
		log.Printf("[GetDatabaseSchema] Error: %v", err)
		InternalError(c, fmt.Sprintf("查询数据库 schema 失败: %v", err))
		return
	}

	OK(c, result)
}

// ========================================
// Knowledge Chat 接口
// ========================================

// KnowledgeStream 知识库对话（带持久化）
func (h *AgentHandler) KnowledgeStream(c *gin.Context) {
	log.Printf("[KnowledgeStream] Received stream request")

	var req agentuc.AgenticRAGRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[KnowledgeStream] Invalid request: %v", err)
		BadRequest(c, err.Error())
		return
	}

	// 设置 agent_id 为 RAG Agent
	req.AgentID = "agent-rag-001"

	// 从上下文获取用户ID和租户ID
	userID := GetUserID(c)
	tenantID := GetTenantID(c)
	log.Printf("[KnowledgeStream] UserID: %d, TenantID: %d, Query: %s", userID, tenantID, req.Query)

	// 设置 SSE 响应头
	sse.SetSSEHeaders(c.Writer)

	// 启动心跳机制（30秒间隔）
	stopHeartbeat := sse.StartHeartbeat(c.Request.Context(), c.Writer, nil)
	defer stopHeartbeat()

	// 收集完整响应用于持久化
	var fullContent strings.Builder
	var toolCalls []agentuc.ToolCallInfo

	// 执行流式聊天（通过持久化服务包装）
	chunkChan, err := h.executeUseCase.ExecuteStream(c.Request.Context(), &req)
	if err != nil {
		log.Printf("[KnowledgeStream] Execution failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	// 发送流式数据并收集内容
	for chunk := range chunkChan {
		if chunk.Done {
			// 发送完成事件，包含完整答案
			sse.SendSSE(c.Writer, "done", gin.H{
				"event":  "done",
				"answer": fullContent.String(),
			})
			break
		}
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
			// 将内容转换为 step 事件（前端期待的格式）
			sse.SendSSE(c.Writer, "step", gin.H{
				"event":   "step",
				"type":    "thought",
				"content": chunk.Content,
			})
		}
		if chunk.Metadata != nil {
			// 检查是否有工具调用事件
			if event, ok := chunk.Metadata["event"].(string); ok {
				switch event {
				case "tool_call":
					if tc, ok := chunk.Metadata["tool_call"].(interface{}); ok {
						sse.SendSSE(c.Writer, "step", gin.H{
							"event":      "step",
							"type":       "tool_result",
							"tool_name":  chunk.Metadata["tool_name"],
							"tool_params": tc,
							"content":    "正在调用工具...",
						})
					}
				case "tool_result":
					if tc, ok := chunk.Metadata["tool_call"].(interface{}); ok {
						// 工具调用信息已在 Agent 层处理
						_ = tc
						sse.SendSSE(c.Writer, "step", gin.H{
							"event":       "step",
							"type":        "tool_result",
							"tool_name":   chunk.Metadata["tool_name"],
							"tool_output": tc,
						})
					}
				default:
					sse.SendSSE(c.Writer, sse.EventTypeMetadata, chunk.Metadata)
				}
			} else {
				if tc, ok := chunk.Metadata["tool_call"].([]agentuc.ToolCallInfo); ok {
					toolCalls = append(toolCalls, tc...)
				}
				sse.SendSSE(c.Writer, sse.EventTypeMetadata, chunk.Metadata)
			}
		}
	}

	// 持久化：保存用户消息和助手回复
	if h.persistenceService != nil && fullContent.Len() > 0 {
		go func() {
			ctx := c.Request.Context()
			persistReq := &agentuc.ExecuteRequest{
				AgentID:   "rag",
				SessionID: req.SessionID,
				Query:     req.Query,
				UserID:    userID,
				TenantID:  tenantID,
			}
			resp := &agentuc.ExecuteResponse{
				Content:    fullContent.String(),
				ToolCalls:  toolCalls,
				AgentSteps: map[string]interface{}{},
			}
			// 使用持久化服务保存（异步）
			_, _ = h.persistenceService.ExecuteWithContext(ctx, persistReq, func(ctx context.Context) (*agentuc.ExecuteResponse, error) {
				return resp, nil
			})
		}()
	}
}
