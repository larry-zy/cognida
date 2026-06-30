// Package agent 提供代理(Agent)领域的领域实体定义
package agent

import (
	"fmt"
	"time"
)

// ========================================
// Agent 相关实体
// ========================================

// AgentType Agent类型
type AgentType string

const (
	AgentTypeNormal      AgentType = "normal"       // 普通聊天 Agent，无工具
	AgentTypeAgenticRAG  AgentType = "agentic_rag"  // RAG 检索 Agent
	AgentTypeDeepResearch AgentType = "deep_research" // 深度研究 Agent
	AgentTypeMultiAgent  AgentType = "multi_agent"  // 多 Agent 协作
)

// IsValid 验证 Agent 类型
func (t AgentType) IsValid() bool {
	switch t {
	case AgentTypeNormal, AgentTypeAgenticRAG, AgentTypeDeepResearch, AgentTypeMultiAgent:
		return true
	}
	return false
}

// ParseAgentType 解析 Agent 类型
func ParseAgentType(s string) (AgentType, error) {
	t := AgentType(s)
	if !t.IsValid() {
		return "", fmt.Errorf("invalid agent type: %s", s)
	}
	return t, nil
}

// AgentStatus Agent状态
type AgentStatus string

const (
	AgentStatusIdle      AgentStatus = "idle"
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusCompleted AgentStatus = "completed"
	AgentStatusFailed    AgentStatus = "failed"
)

// CanExecute 判断是否可以执行
func (s AgentStatus) CanExecute() bool {
	return s == AgentStatusIdle
}

// Agent Agent实体
type Agent struct {
	ID          string
	Name        string
	Description string
	Type        AgentType
	Status      AgentStatus
	Config      *AgentConfig
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewAgent 创建新的 Agent
func NewAgent(name string, agentType AgentType, config *AgentConfig) (*Agent, error) {
	if name == "" {
		return nil, fmt.Errorf("agent name cannot be empty")
	}
	if !agentType.IsValid() {
		return nil, fmt.Errorf("invalid agent type: %s", agentType)
	}
	if config == nil {
		config = DefaultAgentConfig()
	}

	return &Agent{
		ID:        generateAgentID(),
		Name:      name,
		Type:      agentType,
		Status:    AgentStatusIdle,
		Config:    config,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// Execute 执行 Agent
func (a *Agent) Execute() error {
	if !a.Status.CanExecute() {
		return fmt.Errorf("agent cannot execute in status: %s", a.Status)
	}
	a.Status = AgentStatusRunning
	a.UpdatedAt = time.Now()
	return nil
}

// Complete 标记完成
func (a *Agent) Complete() {
	a.Status = AgentStatusCompleted
	a.UpdatedAt = time.Now()
}

// Fail 标记失败
func (a *Agent) Fail(err error) {
	a.Status = AgentStatusFailed
	a.UpdatedAt = time.Now()
}

// Reset 重置状态
func (a *Agent) Reset() {
	a.Status = AgentStatusIdle
	a.UpdatedAt = time.Now()
}

// UpdateConfig 更新配置
func (a *Agent) UpdateConfig(config *AgentConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}
	a.Config = config
	a.UpdatedAt = time.Now()
	return nil
}

// ========================================
// Tool 工具相关
// ========================================

// ToolType 工具类型
type ToolType string

const (
	ToolTypeSearch    ToolType = "search"
	ToolTypeRetrieval ToolType = "retrieval"
	ToolTypeUtility   ToolType = "utility"
	ToolTypeAgent     ToolType = "agent_call"
)

// Tool 工具实体
type Tool struct {
	ID          string
	Name        string
	Description string
	Type        ToolType
	Config      *ToolConfig
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewTool 创建新工具
func NewTool(name string, toolType ToolType) (*Tool, error) {
	if name == "" {
		return nil, fmt.Errorf("tool name cannot be empty")
	}
	return &Tool{
		ID:        generateToolID(),
		Name:      name,
		Type:      toolType,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// Enable 启用工具
func (t *Tool) Enable() {
	t.Enabled = true
	t.UpdatedAt = time.Now()
}

// Disable 禁用工具
func (t *Tool) Disable() {
	t.Enabled = false
	t.UpdatedAt = time.Now()
}

// ToolConfig 工具配置
type ToolConfig struct {
	// 工具特定配置
	Params map[string]interface{}
}

// ========================================
// 辅助函数
// ========================================

func generateAgentID() string {
	return fmt.Sprintf("agent-%d", time.Now().UnixNano())
}

func generateToolID() string {
	return fmt.Sprintf("tool-%d", time.Now().UnixNano())
}
