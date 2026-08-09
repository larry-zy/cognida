// Package handler 提供 Agent 注册中心的 HTTP 处理器
package handler

import (
	"log"
	"strings"

	"github.com/gin-gonic/gin"

	agentdomain "cognida/internal/model/agent"
)

// RegistryAgentHandler Agent 注册中心处理器
type RegistryAgentHandler struct {
	registry agentdomain.AgentRegistry
}

// NewRegistryAgentHandler 创建注册中心 Agent 处理器
func NewRegistryAgentHandler(registry agentdomain.AgentRegistry) *RegistryAgentHandler {
	return &RegistryAgentHandler{
		registry: registry,
	}
}

// containsIgnoreCase 不区分大小写的字符串包含检查
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// ========================================
// Agent 发现接口
// ========================================

// ListAgents 列出所有已注册的 Agent
func (h *RegistryAgentHandler) ListAgents(c *gin.Context) {
	agents, err := h.registry.List(c.Request.Context())
	if err != nil {
		log.Printf("[ListAgents] Failed to list: %v", err)
		InternalError(c, err.Error())
		return
	}

	// 转换为响应格式
	result := make([]gin.H, 0, len(agents))
	for _, agent := range agents {
		result = append(result, gin.H{
			"id":          agent.ID,
			"name":        agent.Name,
			"description": agent.Description,
			"type":        string(agent.Type),
			"status":      string(agent.Status),
			"metadata":    agent.Metadata,
		})
	}

	OK(c, gin.H{
		"count":  len(result),
		"agents": result,
	})
}

// GetAgent 获取指定 Agent 的详细信息
func (h *RegistryAgentHandler) GetAgent(c *gin.Context) {
	agentID := c.Param("id")
	if agentID == "" {
		BadRequest(c, "agent ID 不能为空")
		return
	}

	agentDef, err := h.registry.Get(c.Request.Context(), agentID)
	if err != nil {
		log.Printf("[GetAgent] Failed to get: %v", err)
		NotFound(c, err.Error())
		return
	}

	OK(c, gin.H{
		"id":            agentDef.ID,
		"name":          agentDef.Name,
		"description":   agentDef.Description,
		"type":          string(agentDef.Type),
		"status":        string(agentDef.Status),
		"config":        agentDef.ConfigJSON,
		"metadata":      agentDef.Metadata,
		"registered_at": agentDef.RegisteredAt,
		"updated_at":    agentDef.UpdatedAt,
	})
}

// FindAgents 搜索 Agent（支持关键词）
func (h *RegistryAgentHandler) FindAgents(c *gin.Context) {
	keyword := c.Query("q")
	if keyword == "" {
		// 没有关键词，返回所有
		h.ListAgents(c)
		return
	}

	// 根据类型筛选
	agentType := c.Query("type")

	var agents []*agentdomain.AgentDefinition
	var err error

	if agentType != "" {
		agents, err = h.registry.FindByType(c.Request.Context(), agentdomain.AgentType(agentType))
	} else {
		// 如果注册中心支持 Search 方法，使用它
		// 这里简化处理，获取所有后在内存中过滤
		agents, err = h.registry.List(c.Request.Context())
	}

	if err != nil {
		log.Printf("[FindAgents] Failed to search: %v", err)
		InternalError(c, err.Error())
		return
	}

	// 过滤 - 实现关键词匹配
	result := make([]gin.H, 0)
	for _, agentDef := range agents {
		// 关键词匹配：检查名称和描述中是否包含关键词
		nameMatch := containsIgnoreCase(agentDef.Name, keyword)
		descMatch := containsIgnoreCase(agentDef.Description, keyword)
		if !nameMatch && !descMatch {
			continue
		}

		result = append(result, gin.H{
			"id":          agentDef.ID,
			"name":        agentDef.Name,
			"description": agentDef.Description,
			"type":        string(agentDef.Type),
			"status":      string(agentDef.Status),
		})
	}

	OK(c, gin.H{
		"count":  len(result),
		"agents": result,
	})
}

// ========================================
// Agent 执行接口
// ========================================

// RegistryChatRequest Registry 聊天请求
type RegistryChatRequest struct {
	Message   string                 `json:"message" binding:"required"`
	SessionID string                 `json:"session_id,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ChatExecute 执行 Agent 聊天（通过 Agent 名称）
func (h *RegistryAgentHandler) ChatExecute(c *gin.Context) {
	agentName := c.Param("name")
	if agentName == "" {
		BadRequest(c, "agent 名称不能为空")
		return
	}

	var req RegistryChatRequest
	if !BindJSON(c, &req) {
		return
	}

	log.Printf("[ChatExecute] Agent: %s, Message: %s", agentName, req.Message)

	// 查找 Agent
	agentDef, err := h.registry.FindByName(c.Request.Context(), agentName)
	if err != nil {
		log.Printf("[ChatExecute] Agent not found: %v", err)
		NotFound(c, "Agent 未找到: "+agentName)
		return
	}

	// Agent 执行需要通过 Agent Service 实现
	// 此处仅返回 Agent 信息，实际执行应使用 /api/v1/agent/chat
	OK(c, gin.H{
		"agent_id":     agentDef.ID,
		"agent_name":   agentDef.Name,
		"agent_type":   string(agentDef.Type),
		"message":      req.Message,
		"note":         "请使用 /api/v1/agent/chat 进行实际的 Agent 执行",
	})
}

// ChatExecuteByID 执行 Agent 聊天（通过 Agent ID）
func (h *RegistryAgentHandler) ChatExecuteByID(c *gin.Context) {
	agentID := c.Param("id")
	if agentID == "" {
		BadRequest(c, "agent ID 不能为空")
		return
	}

	var req RegistryChatRequest
	if !BindJSON(c, &req) {
		return
	}

	log.Printf("[ChatExecuteByID] AgentID: %s, Message: %s", agentID, req.Message)

	// 查找 Agent
	agentDef, err := h.registry.Get(c.Request.Context(), agentID)
	if err != nil {
		log.Printf("[ChatExecuteByID] Agent not found: %v", err)
		NotFound(c, "Agent 未找到: "+agentID)
		return
	}

	// Agent 执行需要通过 Agent Service 实现
	// 此处仅返回 Agent 信息，实际执行应使用 /api/v1/agent/chat
	OK(c, gin.H{
		"agent_id":     agentDef.ID,
		"agent_name":   agentDef.Name,
		"agent_type":   string(agentDef.Type),
		"message":      req.Message,
		"note":         "请使用 /api/v1/agent/chat 进行实际的 Agent 执行",
	})
}
