// Package handler 提供 LLM 模型管理的 HTTP 处理器
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	chatapp "link/internal/service/chat"
	domainllm "link/internal/model/llm"
	"link/internal/handler/sse"
)

// ========================================
// Model Handler
// ========================================

// ModelHandler 模型处理器
type ModelHandler struct {
	modelService *chatapp.ModelService
	chatService  *chatapp.ChatService
}

// NewModelHandler 创建模型处理器
func NewModelHandler(
	modelService *chatapp.ModelService,
	chatService *chatapp.ChatService,
) *ModelHandler {
	return &ModelHandler{
		modelService: modelService,
		chatService:  chatService,
	}
}

// ========================================
// 模型管理接口
// ========================================

// CreateModel 创建模型配置
func (h *ModelHandler) CreateModel(c *gin.Context) {
	var req chatapp.CreateModelRequestDTO
	if !BindJSON(c, &req) {
		return
	}

	// 从上下文获取租户ID
	tenantID := GetTenantID(c)

	result, err := h.modelService.CreateModel(c.Request.Context(), tenantID, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Created(c, result)
}

// UpdateModel 更新模型配置
func (h *ModelHandler) UpdateModel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "无效的模型ID")
		return
	}

	var req chatapp.UpdateModelRequestDTO
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.modelService.UpdateModel(c.Request.Context(), id, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// DeleteModel 删除模型配置
func (h *ModelHandler) DeleteModel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "无效的模型ID")
		return
	}

	if err := h.modelService.DeleteModel(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "删除成功"})
}

// GetModel 获取模型配置
func (h *ModelHandler) GetModel(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "无效的模型ID")
		return
	}

	result, err := h.modelService.GetModel(c.Request.Context(), id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ListModels 列出模型配置
func (h *ModelHandler) ListModels(c *gin.Context) {
	tenantID := GetTenantID(c)

	var req chatapp.ListModelsRequestDTO
	req.ModelType = c.Query("model_type")
	page, pageSize := GetPageParams(c)
	req.Page = page
	req.PageSize = pageSize

	if enabled := c.Query("enabled"); enabled != "" {
		if enabled == "true" {
			enabledTrue := true
			req.Enabled = &enabledTrue
		} else if enabled == "false" {
			enabledFalse := false
			req.Enabled = &enabledFalse
		}
	}

	result, err := h.modelService.ListModels(c.Request.Context(), tenantID, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// GetDefaultModel 获取默认模型
func (h *ModelHandler) GetDefaultModel(c *gin.Context) {
	tenantID := GetTenantID(c)

	modelType := c.Query("model_type")
	if modelType == "" {
		modelType = "chat"
	}

	result, err := h.modelService.GetDefaultModel(c.Request.Context(), tenantID, modelType)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ========================================
// 聊天接口
// ========================================

// Chat LLM 聊天
func (h *ModelHandler) Chat(c *gin.Context) {
	var req domainllm.ChatRequest
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.chatService.Chat(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ChatStream LLM 流式聊天
func (h *ModelHandler) ChatStream(c *gin.Context) {
	var req domainllm.ChatRequest
	if !BindJSON(c, &req) {
		return
	}

	chunkChan, err := h.chatService.ChatStream(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	// 设置 SSE 响应头
	sse.SetSSEHeaders(c.Writer)

	// 启动心跳机制
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
	}
}

// GetModelInfo 获取模型信息
func (h *ModelHandler) GetModelInfo(c *gin.Context) {
	info, err := h.chatService.GetModelInfo(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, info)
}
