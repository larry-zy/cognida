// Package agent 提供 Agent 协作相关的领域实体
package agent

import "time"

// ========================================
// CollaborationContextMode 协作上下文模式
// ========================================

// CollaborationContextMode 协作上下文传递模式
type CollaborationContextMode string

const (
	// ContextModeNone 无上下文模式，只传递当前任务
	ContextModeNone CollaborationContextMode = "none"
	// ContextModeSummary 摘要模式，传递对话摘要和委派链路
	ContextModeSummary CollaborationContextMode = "summary"
	// ContextModeRecent 最近消息模式，传递最近 N 条消息
	ContextModeRecent CollaborationContextMode = "recent"
	// ContextModeFull 完整历史模式，传递完整会话历史
	ContextModeFull CollaborationContextMode = "full"
	// ContextModeIsolated 隔离模式，完全独立的会话
	ContextModeIsolated CollaborationContextMode = "isolated"
)

// IsValid 验证模式是否有效
func (m CollaborationContextMode) IsValid() bool {
	switch m {
	case ContextModeNone, ContextModeSummary, ContextModeRecent, ContextModeFull, ContextModeIsolated:
		return true
	}
	return false
}

// ========================================
// CollaborationContext 协作上下文
// ========================================

// CollaborationContext 协作上下文
// 在 Agent 之间传递的共享状态，包含会话信息、委派链路、共享结果等
type CollaborationContext struct {
	// 基础信息
	SessionID     string                 // 会话 ID
	TenantID      int64                  // 租户 ID
	UserID        int64                  // 用户 ID
	OriginalQuery string                 // 用户原始问题

	// 模式配置
	Mode          CollaborationContextMode // 上下文传递模式
	ContextLimit  int                    // recent 模式下的消息数量

	// 委派链路追踪
	DelegateChain []string               // 已委派的 Agent 名称列表

	// 共享状态
	SharedResults map[string]*TaskResult // 各 Agent 的执行结果
	Summary       string                 // 对话摘要

	// 扩展元数据
	Metadata      map[string]interface{} // 扩展元数据

	// 配置
	MaxDepth      int                    // 最大委派深度
	CreatedAt     time.Time              // 创建时间
}

// TaskResult Agent 执行结果
type TaskResult struct {
	AgentName string                 // Agent 名称
	Content   string                 // 响应内容
	Duration  time.Duration          // 执行时长
	Error     error                  // 错误信息
	Metadata  map[string]interface{} // 扩展元数据
	StartTime time.Time              // 开始时间
	EndTime   time.Time              // 结束时间
}

// NewCollaborationContext 创建新的协作上下文
func NewCollaborationContext(sessionID string, tenantID int64, originalQuery string) *CollaborationContext {
	return &CollaborationContext{
		SessionID:     sessionID,
		TenantID:      tenantID,
		OriginalQuery: originalQuery,
		Mode:          ContextModeSummary, // 默认摘要模式
		ContextLimit:  5,                  // 默认最近 5 条
		DelegateChain: make([]string, 0),
		SharedResults: make(map[string]*TaskResult),
		Metadata:      make(map[string]interface{}),
		MaxDepth:      10,                 // 默认最大深度 10
		CreatedAt:     time.Now(),
	}
}

// ========================================
// CollaborationContext 方法
// ========================================

// IsCyclic 检测委派是否形成循环
// 如果目标 Agent 已经在委派链路中，则形成循环
func (c *CollaborationContext) IsCyclic(agentName string) bool {
	for _, name := range c.DelegateChain {
		if name == agentName {
			return true
		}
	}
	return false
}

// AddDelegate 添加委派到链路
func (c *CollaborationContext) AddDelegate(agentName string) {
	c.DelegateChain = append(c.DelegateChain, agentName)
}

// GetDepth 获取当前委派深度
func (c *CollaborationContext) GetDepth() int {
	return len(c.DelegateChain)
}

// IsDepthExceeded 检查是否超过最大深度
func (c *CollaborationContext) IsDepthExceeded() bool {
	return c.MaxDepth > 0 && c.GetDepth() >= c.MaxDepth
}

// ChainDescription 获取委派链路描述
func (c *CollaborationContext) ChainDescription() string {
	if len(c.DelegateChain) == 0 {
		return "无"
	}
	// 构建 "A → B → C" 格式的描述
	description := ""
	for i, name := range c.DelegateChain {
		if i > 0 {
			description += " → "
		}
		description += name
	}
	return description
}

// StoreResult 存储 Agent 执行结果
func (c *CollaborationContext) StoreResult(agentName string, result *TaskResult) {
	if c.SharedResults == nil {
		c.SharedResults = make(map[string]*TaskResult)
	}
	c.SharedResults[agentName] = result
}

// GetResult 获取 Agent 执行结果
func (c *CollaborationContext) GetResult(agentName string) (*TaskResult, bool) {
	if c.SharedResults == nil {
		return nil, false
	}
	result, ok := c.SharedResults[agentName]
	return result, ok
}

// GetAllResults 获取所有执行结果
func (c *CollaborationContext) GetAllResults() map[string]*TaskResult {
	if c.SharedResults == nil {
		return make(map[string]*TaskResult)
	}
	return c.SharedResults
}

// UpdateSummary 更新对话摘要
func (c *CollaborationContext) UpdateSummary(summary string) {
	c.Summary = summary
}

// AppendToSummary 追加到摘要
func (c *CollaborationContext) AppendToSummary(text string) {
	if c.Summary == "" {
		c.Summary = text
	} else {
		c.Summary += "\n" + text
	}
}

// SetMetadata 设置元数据
func (c *CollaborationContext) SetMetadata(key string, value interface{}) {
	if c.Metadata == nil {
		c.Metadata = make(map[string]interface{})
	}
	c.Metadata[key] = value
}

// GetMetadata 获取元数据
func (c *CollaborationContext) GetMetadata(key string) (interface{}, bool) {
	if c.Metadata == nil {
		return nil, false
	}
	value, ok := c.Metadata[key]
	return value, ok
}

// Clone 克隆协作上下文
func (c *CollaborationContext) Clone() *CollaborationContext {
	clone := &CollaborationContext{
		SessionID:     c.SessionID,
		TenantID:      c.TenantID,
		UserID:        c.UserID,
		OriginalQuery: c.OriginalQuery,
		Mode:          c.Mode,
		ContextLimit:  c.ContextLimit,
		MaxDepth:      c.MaxDepth,
		CreatedAt:     c.CreatedAt,
	}

	// 克隆委派链路
	if c.DelegateChain != nil {
		clone.DelegateChain = make([]string, len(c.DelegateChain))
		copy(clone.DelegateChain, c.DelegateChain)
	}

	// 克隆摘要
	clone.Summary = c.Summary

	// 克隆共享结果
	if c.SharedResults != nil {
		clone.SharedResults = make(map[string]*TaskResult)
		for k, v := range c.SharedResults {
			clone.SharedResults[k] = v
		}
	}

	// 克隆元数据
	if c.Metadata != nil {
		clone.Metadata = make(map[string]interface{})
		for k, v := range c.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}
