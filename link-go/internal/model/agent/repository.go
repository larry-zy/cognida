// Package agent 提供代理(Agent)领域的仓储接口定义
package agent

import "context"

// ========================================
// AgentRepository Agent仓储接口
// ========================================

// AgentRepository Agent仓储接口
// 注意：Agent主要运行时存在，此接口主要用于持久化Agent配置和执行记录
type AgentRepository interface {
	// SaveConfig 保存Agent配置
	SaveConfig(ctx context.Context, agentID string, config interface{}) error

	// LoadConfig 加载Agent配置
	LoadConfig(ctx context.Context, agentID string) (interface{}, error)

	// SaveExecutionRecord 保存执行记录
	SaveExecutionRecord(ctx context.Context, record *ExecutionRecord) error

	// FindExecutionRecords 查找执行记录
	FindExecutionRecords(ctx context.Context, sessionID string, page, pageSize int) ([]*ExecutionRecord, int64, error)
}

// ExecutionRecord Agent执行记录
type ExecutionRecord struct {
	ID         string
	AgentID    string
	SessionID  string
	Query      string
	Answer     string
	Success    bool
	Error      string
	Duration   int64
	ToolCalls  string
	Metadata   string
	CreatedAt  int64
}
