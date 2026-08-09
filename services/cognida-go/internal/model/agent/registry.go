// Package agent 提供 Agent 注册中心接口
package agent

import (
	"context"
)

// ========================================
// Agent 注册中心接口
// ========================================

// AgentRegistry Agent 注册中心接口
// 用于运行时注册、发现和获取 Agent 实例
type AgentRegistry interface {
	// Register 注册一个 Agent
	Register(ctx context.Context, agent *AgentDefinition) error

	// Unregister 注销一个 Agent
	Unregister(ctx context.Context, agentID string) error

	// Get 获取指定的 Agent
	Get(ctx context.Context, agentID string) (*AgentDefinition, error)

	// List 列出所有已注册的 Agent
	List(ctx context.Context) ([]*AgentDefinition, error)

	// FindByType 根据类型查找 Agent
	FindByType(ctx context.Context, agentType AgentType) ([]*AgentDefinition, error)

	// FindByName 根据名称查找 Agent
	FindByName(ctx context.Context, name string) (*AgentDefinition, error)

	// Exists 检查 Agent 是否存在
	Exists(ctx context.Context, agentID string) (bool, error)

	// UpdateStatus 更新 Agent 状态
	UpdateStatus(ctx context.Context, agentID string, status AgentStatus) error
}

// AgentDefinition Agent 定义（运行时）
// 这是注册中心存储的 Agent 元信息，实际 Agent 实例由 Factory 创建
type AgentDefinition struct {
	// 基本信息
	ID          string
	Name        string
	Description string
	Type        AgentType
	Status      AgentStatus

	// 配置（JSON 序列化）
	ConfigJSON string

	// 元数据
	Metadata map[string]string

	// 注册时间
	RegisteredAt int64
	UpdatedAt    int64
}

// AgentFactory Agent 工厂接口
// 用于根据 AgentDefinition 创建实际的 Agent 实例
type AgentFactory interface {
	// CreateAgent 创建 Agent 实例
	CreateAgent(ctx context.Context, definition *AgentDefinition) (interface{}, error)
}

// AgentExecutable 可执行的 Agent 接口
// 所有通过注册中心获取的 Agent 都需要实现此接口
type AgentExecutable interface {
	// Chat 执行聊天
	Chat(ctx context.Context, message string, opts ...ChatOption) (*ChatResponse, error)

	// Stream 流式聊天
	Stream(ctx context.Context, message string, opts ...ChatOption) (<-chan *ChatChunk, error)
}

// ChatOption 聊天选项
type ChatOption func(*ChatOptions)

// ChatOptions 聊天配置
type ChatOptions struct {
	SessionID string
	UserID    string
	Metadata  map[string]interface{}
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Content   string
	ToolCalls []ToolCallResult
	Metadata  map[string]interface{}
}

// ToolCallResult 工具调用结果
type ToolCallResult struct {
	Name   string
	Input  map[string]interface{}
	Output string
	Error  error
}

// ChatChunk 流式响应块
type ChatChunk struct {
	Content string
	Done    bool
	Metadata map[string]interface{}
}
