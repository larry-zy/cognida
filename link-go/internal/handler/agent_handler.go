// Package handler 提供 Agent 相关的 HTTP 处理器
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	agentctx "link/internal/model/agent"
	"link/internal/model/agent/operations"
	agentuc "link/internal/service/agent"
	"link/internal/service/agent/agentstate"
	"link/internal/service/agent/genui"
	"link/internal/service/agent/pendingaction"
	dataagent "link/internal/service/agent/presets/data_agent"
	"link/internal/service/agent/resultstore"
	ragtool "link/internal/service/agent/tools"
	"link/internal/service/agent/uibinding"
	"link/internal/handler/sse"
)

// resolveRequestID 复用 TraceMiddleware 注入的 request_id，实现端到端链路一致：
// HTTP → agent 编排 → gRPC 出站 → Python 共享同一 ID。仅当上游缺失（理论上不应发生，
// 如非经中间件的内部调用）时才回退生成带前缀的新 ID，保留调用场景可读性。
func resolveRequestID(ctx context.Context, prefix string) string {
	if rid, ok := agentctx.GetRequestID(ctx); ok && rid != "" {
		return rid
	}
	return fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
}

// genUIOption 控制流式过程中是否装配并下发生成式 UI（A2UI）契约。
//
// 渲染即工具（Phase 3）后，`ui` 事件的主路径是 render_ui 工具：agent 每次调用
// render_ui，handler 从 tool_result 中提取校验后的 UISpec 并立即下发（一次回答
// 可有多个独立 surface）。compose 为 true 时保留兜底：整个流未产生任何 render_ui
// surface 才在 done 之前用捕获的 sql_execute / data_analysis 输出做一次性拼装
// （兼容未挂载 render_ui 的旧 preset，Phase 8 切换后可移除）。
type genUIOption struct {
	compose   bool
	question  string
	sessionID string // 归属会话：Confirm 卡片 resume 回调需回传（Phase 5 任务 6.3）
}

// ========================================
// Agent Handler
// ========================================

// agentToolGateway 是 handler 侧访问工具注册表能力的窄接口（决策A）：组合根经
// SetToolGateway 注入具体 *tools.ToolRegistry，handler 只依赖这几个 confirm-resume /
// UI 取数 / schema 查询所需方法，不再经包级默认槽位（default.go）读取，也不泄露
// 整个 ToolRegistry 具体类型到 handler 层。
type agentToolGateway interface {
	ResultStore() resultstore.Store
	PendingStore() pendingaction.Store
	UIBinding() uibinding.Store
	// SessionState 按 (tenant, session) 装配会话态门面（薄门面 AgentState）：
	// UI 回调等会话态读写经它统一 owner 归属，替代散在各处的手拼 OwnerKey + 直取 store。
	SessionState(tenantID int64, sessionID string) *agentstate.AgentState
	ExecuteConfirmedMutation(ctx context.Context, action *pendingaction.PendingAction) (*ragtool.SQLMutateResult, error)
	ExecuteConfirmedETL(ctx context.Context, action *pendingaction.PendingAction) (*ragtool.ETLRunResult, error)
	ExecuteConfirmedExport(ctx context.Context, action *pendingaction.PendingAction) (*ragtool.DataExportResult, error)
	RecordUnsupportedConfirmKind(ctx context.Context, action *pendingaction.PendingAction)
	FetchSchema(ctx context.Context, databaseID, tableName string) (*ragtool.GetSchemaResult, error)
}

// AgentHandler Agent 处理器
type AgentHandler struct {
	executeUseCase       *agentuc.ExecuteService
	researchUseCase      *agentuc.ResearchService
	configUseCase         *agentuc.ConfigService
	progressUseCase       *agentuc.ProgressService
	persistenceService   *agentuc.AgentPersistenceService
	// toolGateway 由组合根注入（SetToolGateway）：confirm-resume / UI 取数 / schema 查询
	// 经它访问工具注册表能力。未注入时相关端点返回未初始化错误（nil-guard）。
	toolGateway agentToolGateway
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

// SetToolGateway 注入工具注册表网关，由组合根（cmd/server）在构造 ToolRegistry 后调用一次。
func (h *AgentHandler) SetToolGateway(gw agentToolGateway) {
	h.toolGateway = gw
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

// agentStreamResult 汇总一次流式执行的可持久化产物。
type agentStreamResult struct {
	Content    string                   // 完整回答文本
	ToolCalls  []agentuc.ToolCallInfo   // 工具调用记录
	Steps      []map[string]interface{} // 结构化步骤轨迹（用于历史回放时间线）
	UISurfaces []*genui.UISpec          // 本次回答渲染的 UI surface（含有界数据快照，随消息持久化）
}

// extractRenderedUISpec 从 render_ui 的 tool_output（完整 JSON）中提取校验后的 UISpec。
// 工具校验失败时 status=error 且无 ui_spec，返回 nil —— 即"校验失败不推 ui 事件"。
func extractRenderedUISpec(toolOutput string) *genui.UISpec {
	if toolOutput == "" {
		return nil
	}
	var out struct {
		UISpec *genui.UISpec `json:"ui_spec"`
	}
	if err := json.Unmarshal([]byte(toolOutput), &out); err != nil {
		return nil
	}
	if out.UISpec == nil || len(out.UISpec.Components) == 0 {
		return nil
	}
	return out.UISpec
}

// extractPendingConfirmUISpec 从 sql_mutate 的 tool_output 中识别危险操作暂停
// （status=pending_confirm），装配确认卡片 surface（Phase 5 任务 6.3）。
// 非暂停结果 / 解析失败返回 nil（不推 ui 事件）。UI 契约在 Go 端确定性拼装。
func extractPendingConfirmUISpec(toolOutput, sessionID string) *genui.UISpec {
	if toolOutput == "" {
		return nil
	}
	var out struct {
		Status          string `json:"status"`
		Target          string `json:"target"`
		RowsAffected    int64  `json:"rows_affected"`
		PendingActionID string `json:"pending_action_id"`
		ConfirmToken    string `json:"confirm_token"`
		Message         string `json:"message"`
	}
	if err := json.Unmarshal([]byte(toolOutput), &out); err != nil {
		return nil
	}
	if out.Status != operations.StatusPendingConfirm || out.PendingActionID == "" {
		return nil
	}
	return genui.ConfirmCompose(genui.ConfirmInput{
		Surface:         "sfc_" + uuid.NewString()[:8],
		Target:          out.Target,
		RowsAffected:    out.RowsAffected,
		Message:         out.Message,
		PendingActionID: out.PendingActionID,
		ConfirmToken:    out.ConfirmToken,
		SessionID:       sessionID,
	})
}

// metaString 从 metadata 中安全取字符串字段。
func metaString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// streamAgentChunks 消费 chunkChan，按统一的结构化 SSE 契约向前端下发：
//   - content 事件      → step{type:"thought", content}（追加到回答气泡）
//   - tool_call 事件    → step{type:"tool_call", step, tool_name, tool_input, status}
//   - tool_result 事件  → step{type:"tool_result", step, tool_name, tool_output, status, error}
//   - error 事件        → step{type:"error", error}
//   - ui                → ui{...UISpec}（render_ui 工具每次成功调用即时下发，可多次）
//   - done              → done{answer}
//
// 同时收集完整回答、工具调用记录、步骤轨迹与 UI surface 用于持久化。
//
// `ui` 事件即时化：tool_result 中 tool_name=="render_ui" 且成功时，立即提取校验后的
// UISpec 下发（不等流结束）；校验失败的调用 status=error、无 ui_spec，不推 ui 事件
// （错误已由 eino 循环回灌 LLM 自纠）。genUI.compose 仅作旧 preset 兜底：整个流
// 没有任何 render_ui surface 时，done 之前用捕获的工具输出一次性拼装。
func (h *AgentHandler) streamAgentChunks(c *gin.Context, chunkChan <-chan *agentuc.ChatChunkDTO, genUI genUIOption) agentStreamResult {
	var sb strings.Builder
	var toolCalls []agentuc.ToolCallInfo
	var steps []map[string]interface{}
	var uiSurfaces []*genui.UISpec
	var sqlOutputs, analysisOutputs []string // 捕获本次全部真实工具输出，供旧路径 genUI 兜底融合

	// 客户端断开（request ctx 取消）即停止消费与下发；上游发送端同样以该 ctx 终止生成。
	ctx := c.Request.Context()

	for {
		var chunk *agentuc.ChatChunkDTO
		var ok bool
		select {
		case chunk, ok = <-chunkChan:
			if !ok {
				return agentStreamResult{Content: sb.String(), ToolCalls: toolCalls, Steps: steps, UISurfaces: uiSurfaces}
			}
		case <-ctx.Done():
			return agentStreamResult{Content: sb.String(), ToolCalls: toolCalls, Steps: steps, UISurfaces: uiSurfaces}
		}

		if chunk.Done {
			// 兜底路径：未产生任何 render_ui surface 时才做 done 前一次性拼装。
			if genUI.compose && len(uiSurfaces) == 0 {
				if spec := genui.Compose(c.Request.Context(), genui.ComposeInput{
					Question:        genUI.question,
					SQLOutputs:      sqlOutputs,
					AnalysisOutputs: analysisOutputs,
				}); spec != nil {
					uiSurfaces = append(uiSurfaces, spec)
					sse.SendSSE(c.Writer, "ui", spec)
				}
			}
			sse.SendSSE(c.Writer, "done", gin.H{
				"event":  "done",
				"answer": sb.String(),
				// 回传归属会话 ID：新会话首轮时前端据此绑定 currentSessionId，
				// 使后续轮次复用同一会话（否则每轮都以空 session_id 新建会话，对话被拆散）。
				"session_id": genUI.sessionID,
			})
			break
		}

		// 文本增量：作为回答内容
		if chunk.Content != "" {
			sb.WriteString(chunk.Content)
			sse.SendSSE(c.Writer, "step", gin.H{
				"event":   "step",
				"type":    "thought",
				"content": chunk.Content,
			})
			continue
		}

		// 结构化步骤事件
		if chunk.Metadata == nil {
			continue
		}
		stepType := metaString(chunk.Metadata["type"])
		if stepType == "" {
			continue
		}

		// 原样透传结构化字段，并标记 SSE 事件名
		payload := gin.H{"event": "step"}
		for k, v := range chunk.Metadata {
			payload[k] = v
		}
		sse.SendSSE(c.Writer, "step", payload)
		steps = append(steps, chunk.Metadata)

		// 工具执行结果收集为持久化记录
		if stepType == "tool_result" {
			toolName := metaString(chunk.Metadata["tool_name"])
			toolOutput := metaString(chunk.Metadata["tool_output"])
			toolErr := metaString(chunk.Metadata["error"])
			toolCalls = append(toolCalls, agentuc.ToolCallInfo{
				ID:     metaString(chunk.Metadata["tool_id"]),
				Name:   toolName,
				Input:  metaString(chunk.Metadata["tool_input"]),
				Output: toolOutput,
				Error:  toolErr,
			})
			switch toolName {
			case "render_ui":
				// 渲染即工具：每次成功调用立即下发独立 ui surface（不等流结束）。
				if toolErr == "" {
					if spec := extractRenderedUISpec(toolOutput); spec != nil {
						uiSurfaces = append(uiSurfaces, spec)
						sse.SendSSE(c.Writer, "ui", spec)
					}
				}
			case "sql_mutate":
				// 危险操作暂停：Go 端确定性拼装确认卡片并即时下发（Phase 5 任务 6.3）。
				// 卡片确认动作携 pending_action_id + token + session_id 回调 confirm 端点。
				if toolErr == "" {
					if spec := extractPendingConfirmUISpec(toolOutput, genUI.sessionID); spec != nil {
						uiSurfaces = append(uiSurfaces, spec)
						sse.SendSSE(c.Writer, "ui", spec)
					}
				}
			// 捕获真实工具输出，供 done 前的旧路径 genUI 兜底（收全部结果集，融合成一份 DataModel）。
			case "sql_execute":
				if toolOutput != "" {
					sqlOutputs = append(sqlOutputs, toolOutput)
				}
			case "data_analysis":
				if toolOutput != "" {
					analysisOutputs = append(analysisOutputs, toolOutput)
				}
			}
		}
	}

	return agentStreamResult{Content: sb.String(), ToolCalls: toolCalls, Steps: steps, UISurfaces: uiSurfaces}
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

	// 发送流式数据（结构化 step 契约）
	h.streamAgentChunks(c, chunkChan, genUIOption{})
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

	// Phase 8 任务 9.1：主入口迁移到 Data Agent（单一 ReAct 内核 + 子代理委派）。
	// 旧 text2sql preset 仍保留注册，显式携 agent_id 的调用方可继续使用（兼容）。
	req.AgentID = dataagent.DataAgentID

	// 从上下文获取用户ID和租户ID
	userID := GetUserID(c)
	tenantID := GetTenantID(c)
	log.Printf("[Text2SQLStream] UserID: %d, TenantID: %d, Query: %s", userID, tenantID, req.Query)

	// 使用持久化服务处理会话和消息（AgentType 仍映射 text2sql，前端会话列表兼容）
	prepareReq := &agentuc.PrepareSessionRequest{
		AgentID:   dataagent.DataAgentID,
		SessionID: req.SessionID,
		UserID:    userID,
		TenantID:  tenantID,
		Query:     req.Query, // 新建会话时据此自动命名（否则所有会话同名）
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

	// 更新请求的 session_id 为准备好的会话 ID
	req.SessionID = session.ID

	// Phase 8 任务 9.1：主入口注入 Agent 上下文——租户/会话/用户/请求 ID（审计留痕）
	// + 协作上下文（子代理委派的循环/深度检测与 Result Store 归属）+ 会话工具 scope。
	// 主入口默认授予最小权限 read：写/ETL 类工具由硬工具门拦截，危险操作走
	// pending-confirm 人工确认流，不在此直接放行。
	ctx := agentctx.NewAgentContext(c.Request.Context(), session.ID, tenantID, userID, req.Query)
	ctx = agentctx.WithRequestID(ctx, resolveRequestID(c.Request.Context(), "t2s"))
	ctx = agentctx.WithToolScope(ctx, "read")
	// 会话数据源上下文：非空时查询类工具默认路由到该外部数据源，
	// 且 sql_mutate/etl_run 被硬拒（外部数据源只读）。空=当前业务库（向后兼容）。
	ctx = agentctx.WithDatasourceID(ctx, req.DatasourceID)

	// 执行流式聊天
	chunkChan, err := h.executeUseCase.ExecuteStream(ctx, &req)
	if err != nil {
		log.Printf("[Text2SQLStream] Execution failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	// 发送流式数据并收集内容（结构化 step 契约）；Text2SQL 开启生成式 UI 融合。
	result := h.streamAgentChunks(c, chunkChan, genUIOption{compose: true, question: req.Query, sessionID: session.ID})

	// 持久化：保存助手消息。
	// 注意：不能只以 Content 非空为条件——Data Agent 可能只产出画布 UI（surfaces）或工具调用而无文本，
	// 若据此跳过落库，会话重开时画布将丢失。因此只要有文本/工具调用/UI surface 任一产出即持久化。
	if h.persistenceService != nil && (result.Content != "" || len(result.UISurfaces) > 0 || len(result.ToolCalls) > 0) {
		assistantMsgID := fmt.Sprintf("msg-%s", uuid.New().String()[:8])
		// 捕获需要的变量
		sessionID := session.ID
		content := result.Content
		toolCalls := result.ToolCalls
		steps := result.Steps
		uiSurfaces := result.UISurfaces
		go func() {
			// 使用新的 context，避免请求 context 被取消
			ctx := context.Background()
			agentSteps := map[string]interface{}{
				"session_id": sessionID,
				"steps":      steps,
			}
			// UI 持久化：A2UI 规格（含有界数据快照）随 assistant 消息落 MySQL，
			// 会话重开从消息记录重现 surface，不依赖 SSE 重放（Phase 3 任务 4.5/4.6）。
			if len(uiSurfaces) > 0 {
				agentSteps["ui_surfaces"] = uiSurfaces
			}
			saveReq := &agentuc.SaveAssistantMessageRequest{
				MessageID:  assistantMsgID,
				SessionID:  sessionID,
				Content:    content,
				ToolCalls:  toolCalls,
				AgentSteps: agentSteps,
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

// ========================================
// 生成式 UI 交互回调（Phase 3 任务 4.7/4.8）
// ========================================

// uiPageSizeMax 单页最大行数（防止一次取回超大结果集）。
const uiPageSizeMax = 200

// GetUISurfacePage 按 surface 绑定状态分页取数（Pagination/Filter 组件回调）。
//
// 路由依据 Redis 中的交互绑定状态（surface ↔ result_id + token）：
//   - 绑定不存在/超会话 TTL → status=session_expired（"会话已过期"，非错误路由）
//   - token 不匹配          → 403（防伪造回调）
//   - result_id 已过期      → status=data_expired（"数据已过期，可重跑"占位降级；
//     小结果的有界快照已随消息持久化，前端直接用快照重现，无需走本接口）
//   - 正常                  → 按 cursor 返回对应页（不重跑查询）
//
// Query 参数：token（必填）、cursor（默认 0）、page_size（默认 50，上限 200）
func (h *AgentHandler) GetUISurfacePage(c *gin.Context) {
	surface := c.Param("surface")
	token := c.Query("token")
	if surface == "" || token == "" {
		BadRequest(c, "surface 与 token 不能为空")
		return
	}

	if h.toolGateway == nil {
		InternalError(c, "工具注册表未注入")
		return
	}
	bindingStore := h.toolGateway.UIBinding()
	if bindingStore == nil {
		InternalError(c, "交互绑定存储未启用")
		return
	}

	binding, err := bindingStore.Get(c.Request.Context(), surface)
	if err != nil {
		// 超会话 TTL：返回"会话已过期"占位，而非错误路由
		OK(c, gin.H{"status": "session_expired", "message": "会话已过期"})
		return
	}
	if binding.Token != token {
		Forbidden(c, "回调 token 不匹配")
		return
	}
	// 租户边界：token 是能力凭证，但绑定归属租户仍必须与认证租户一致，
	// 防止凭 surface+token 跨租户读取 Result Store（同 token 不匹配文案，不泄露存在性）。
	if binding.TenantID != GetTenantID(c) {
		Forbidden(c, "回调 token 不匹配")
		return
	}
	if binding.ResultID == "" {
		OK(c, gin.H{"status": "data_expired", "message": "数据已过期，可重跑"})
		return
	}

	// 经会话态门面按 owner(tenant:session) 读回结果集：门面统一归属键，UI 回调
	// 不再手拼 OwnerKey + 直取 ResultStore。
	st := h.toolGateway.SessionState(binding.TenantID, binding.SessionID)
	store := st.Results()
	if store == nil {
		InternalError(c, "结果存储未启用")
		return
	}

	result, err := store.Get(c.Request.Context(), st.OwnerKey(), binding.ResultID)
	if err != nil {
		// Result Store TTL 过期（或归属异常）：大数据降级占位（4.7）
		OK(c, gin.H{"status": "data_expired", "message": "数据已过期，可重跑"})
		return
	}

	// cursor 分页（约定：分页使用 cursor 而非 offset 语义暴露；cursor 为行位置）
	cursor, _ := strconv.Atoi(c.DefaultQuery("cursor", "0"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if cursor < 0 {
		cursor = 0
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > uiPageSizeMax {
		pageSize = uiPageSizeMax
	}

	total := len(result.Rows)
	if cursor > total {
		cursor = total
	}
	end := cursor + pageSize
	if end > total {
		end = total
	}
	nextCursor := ""
	if end < total {
		nextCursor = strconv.Itoa(end)
	}

	OK(c, gin.H{
		"status":      "ok",
		"result_id":   binding.ResultID,
		"columns":     result.Columns,
		"rows":        result.Rows[cursor:end],
		"row_count":   total,
		"cursor":      cursor,
		"next_cursor": nextCursor,
	})
}

// ========================================
// 危险操作人机确认 resume（Phase 5 任务 6.2）
// ========================================

// confirmOperationRequest 确认请求：pending_action_id + 一次性 token + 所属会话。
type confirmOperationRequest struct {
	PendingActionID string `json:"pending_action_id" binding:"required"`
	Token           string `json:"token" binding:"required"`
	SessionID       string `json:"session_id" binding:"required"`
}

// ConfirmOperation 危险操作确认 resume 端点。
//
// sql_mutate 危险级操作（影响行数 ≥ 阈值）会在事务内 dry-run 后回滚并暂存为
// pending action，前端确认卡片携 pending_action_id + token 调本端点恢复执行：
//   - token 匹配且未过期 → 提交事务（幂等与审计仍生效）
//   - token 不匹配        → 403 拒绝，且该 pending action 立即失效（防暴力尝试）
//   - 不存在/已过期/归属不符 → status=expired（非错误路由，前端展示"已过期"占位，
//     不泄露他人 pending action 的存在性）
func (h *AgentHandler) ConfirmOperation(c *gin.Context) {
	var req confirmOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if h.toolGateway == nil {
		InternalError(c, "工具注册表未注入")
		return
	}
	store := h.toolGateway.PendingStore()
	if store == nil {
		InternalError(c, "待确认操作存储未启用")
		return
	}

	// 归属键与暂存时一致：tenant 来自认证上下文，session 由确认卡片回传
	tenantID := GetTenantID(c)
	owner := resultstore.OwnerKey(tenantID, req.SessionID)

	action, err := store.Consume(c.Request.Context(), owner, req.PendingActionID, req.Token)
	if err != nil {
		if errors.Is(err, pendingaction.ErrTokenMismatch) {
			// token 不匹配：拒绝，且该 pending action 已在 Consume 内消费失效
			Forbidden(c, "确认 token 不匹配，该待确认操作已失效")
			return
		}
		OK(c, gin.H{"status": "expired", "message": "待确认操作不存在或已过期，请重新发起"})
		return
	}

	// 注入 agent 上下文：确认执行的审计沿用原 tenant/session 归属，
	// 并携认证 UserID——审计须可追溯是谁批准了危险操作。
	ctx := agentctx.WithTenantID(c.Request.Context(), tenantID)
	ctx = agentctx.WithSessionID(ctx, req.SessionID)
	ctx = agentctx.WithUserID(ctx, GetUserID(c))
	ctx = agentctx.WithRequestID(ctx, resolveRequestID(c.Request.Context(), "confirm"))

	// 按 pending action 类型分派恢复执行：写（危险级/策略级审批）、ETL、导出均走既有
	// confirm-resume 通道；策略级审批的 etl/export 与危险级写共用同一确认端点（任务 4.3）。
	var (
		result  interface{}
		execErr error
	)
	switch action.Kind {
	case operations.OpMutate:
		result, execErr = h.toolGateway.ExecuteConfirmedMutation(ctx, action)
	case operations.OpETL:
		result, execErr = h.toolGateway.ExecuteConfirmedETL(ctx, action)
	case operations.OpExport:
		result, execErr = h.toolGateway.ExecuteConfirmedExport(ctx, action)
	default:
		// pending action 消费即失效：进不了执行分支也必须留痕，防静默丢弃
		h.toolGateway.RecordUnsupportedConfirmKind(ctx, action)
		BadRequest(c, fmt.Sprintf("不支持的待确认操作类型: %s", action.Kind))
		return
	}
	if execErr != nil {
		InternalError(c, execErr.Error())
		return
	}
	OK(c, result)
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

	if h.toolGateway == nil {
		InternalError(c, "工具注册表未注入")
		return
	}
	// database_id 非空时按外部数据源路由，需要租户上下文做归属校验
	ctx := agentctx.WithTenantID(c.Request.Context(), GetTenantID(c))
	result, err := h.toolGateway.FetchSchema(ctx, databaseID, tableName)
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
	log.Printf("[KnowledgeStream] UserID: %d, TenantID: %d, KBIDs: %v, GraphEnabled: %v, KBScopeMode: %q, Query: %s",
		userID, tenantID, req.KBIDs, req.GraphEnabled, req.KBScopeMode, req.Query)

	// 准备会话并先落库用户消息（须在执行前，使记忆分支能回放到含本轮的历史，
	// 且拿到规范化 session.ID 注入执行上下文）。与 Data Agent 主入口一致。
	if h.persistenceService != nil {
		session, err := h.persistenceService.PrepareSession(c.Request.Context(), &agentuc.PrepareSessionRequest{
			AgentID:   req.AgentID,
			SessionID: req.SessionID,
			UserID:    userID,
			TenantID:  tenantID,
			Query:     req.Query, // 新建会话时据此自动命名（否则所有会话同名）
		})
		if err != nil {
			log.Printf("[KnowledgeStream] Prepare session failed: %v", err)
			InternalError(c, err.Error())
			return
		}
		req.SessionID = session.ID
		userMsgID := fmt.Sprintf("msg-%s", uuid.New().String()[:8])
		if err := h.persistenceService.SaveUserMessage(c.Request.Context(), &agentuc.SaveMessageRequest{
			MessageID: userMsgID,
			SessionID: session.ID,
			Content:   req.Query,
			UserID:    userID,
		}); err != nil {
			log.Printf("[KnowledgeStream] Save user message failed: %v", err) // 不中断
		}
	}

	// 结构化透传：把租户/用户/知识库范围/图谱开关/选择模式注入 Go context，
	// 供下游 Agent 工具层读取（scope 强制、多 KB 检索、图谱门控、agentic 选库），
	// 替代旧的“文本前缀塞进 query”方案。
	ctx := c.Request.Context()
	// 注入会话 ID：RAG Agent 的记忆分支据此从 messages 表回放历史，支持多轮追问。
	ctx = agentctx.WithSessionID(ctx, req.SessionID)
	ctx = agentctx.WithTenantID(ctx, tenantID)
	ctx = agentctx.WithUserID(ctx, userID)
	ctx = agentctx.WithAllowedKBIDs(ctx, req.KBIDs)
	ctx = agentctx.WithGraphEnabled(ctx, req.GraphEnabled)
	// 选择模式 + 路由 holder：结合/智能模式下，kb_route 工具把 AI 的选库写入该 holder，
	// 供其后的 rag_query/graph_query 读取（跨工具调用共享同一指针）。
	ctx = agentctx.WithKBScopeMode(ctx, req.KBScopeMode)
	ctx = agentctx.WithRouteSelection(ctx, agentctx.NewRouteSelection())

	// 设置 SSE 响应头
	sse.SetSSEHeaders(c.Writer)

	// 启动心跳机制（30秒间隔）
	stopHeartbeat := sse.StartHeartbeat(ctx, c.Writer, nil)
	defer stopHeartbeat()

	// 执行流式聊天（通过持久化服务包装）
	chunkChan, err := h.executeUseCase.ExecuteStream(ctx, &req)
	if err != nil {
		log.Printf("[KnowledgeStream] Execution failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	// 发送流式数据并收集内容（结构化 step 契约）；回传归属会话 ID 供前端绑定新会话。
	result := h.streamAgentChunks(c, chunkChan, genUIOption{sessionID: req.SessionID})

	// 持久化：仅保存助手回复（用户消息已在执行前落库，避免重复）。
	if h.persistenceService != nil && (result.Content != "" || len(result.UISurfaces) > 0 || len(result.ToolCalls) > 0) {
		assistantMsgID := fmt.Sprintf("msg-%s", uuid.New().String()[:8])
		sessionID := req.SessionID
		content := result.Content
		toolCalls := result.ToolCalls
		steps := result.Steps
		uiSurfaces := result.UISurfaces
		go func() {
			// 使用独立 context，避免 SSE 响应结束后请求 context 被取消导致落库失败
			ctx := context.Background()
			agentSteps := map[string]interface{}{
				"session_id": sessionID,
				"steps":      steps,
			}
			if len(uiSurfaces) > 0 {
				agentSteps["ui_surfaces"] = uiSurfaces
			}
			saveReq := &agentuc.SaveAssistantMessageRequest{
				MessageID:  assistantMsgID,
				SessionID:  sessionID,
				Content:    content,
				ToolCalls:  toolCalls,
				AgentSteps: agentSteps,
				TokenCount: 0,
			}
			if err := h.persistenceService.SaveAssistantMessage(ctx, saveReq); err != nil {
				log.Printf("[KnowledgeStream] Failed to save assistant message: %v", err)
			}
		}()
	}
}
