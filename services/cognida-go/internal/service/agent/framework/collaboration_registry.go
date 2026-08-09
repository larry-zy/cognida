// Package agent provides collaboration registry for multi-agent runtime management.
package framework

import (
	"fmt"
	"sync"

	domainagent "cognida/internal/model/agent"
)

// ========================================
// Collaboration Registry (多 Agent 协作注册表)
// ========================================

// AgentCapability represents an agent's capability.
type AgentCapability struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills"`
}

// AgentInfo represents information about an agent.
type AgentInfo struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Capabilities []AgentCapability `json:"capabilities"`
}

// AgentGovernance 子代理治理元数据（Phase 7）：构成活体 agent 目录，
// 供审计留痕（agent_operation_audit 串联）与最小权限校验。
type AgentGovernance struct {
	// Purpose 用途描述（该子代理解决什么问题）。
	Purpose string `json:"purpose"`
	// DataScope 数据访问级（如 "业务库只读" / "agent_etl_* 派生表读写"）。
	DataScope string `json:"data_scope"`
	// Tools 声明的最小工具集（工具名列表，注册时固定）。
	Tools []string `json:"tools"`
	// RiskClass 风险级：read / write / etl（与会话 scope 阶梯同构）。
	RiskClass string `json:"risk_class"`
}

// AgentEntry represents a registered agent.
type AgentEntry struct {
	Agent        Agent
	Capabilities []AgentCapability
	Description  string
	// Governance 治理元数据（Phase 7）；nil 表示未声明（旧注册路径）。
	Governance *AgentGovernance
	// ContextMode 该子代理被委派时的默认上下文模式（上下文防火墙）；
	// 空值按 summary 处理。探查/写作类应为 isolated。
	ContextMode domainagent.CollaborationContextMode
}

// CollaborationRegistry manages available agents and their capabilities for multi-agent collaboration.
type CollaborationRegistry struct {
	mu     sync.RWMutex
	agents map[string]*AgentEntry
}

// NewCollaborationRegistry creates a new collaboration registry.
func NewCollaborationRegistry() *CollaborationRegistry {
	return &CollaborationRegistry{
		agents: make(map[string]*AgentEntry),
	}
}

// Register registers an agent with its capabilities.
func (r *CollaborationRegistry) Register(id string, agent Agent, capabilities []AgentCapability, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.agents[id] = &AgentEntry{
		Agent:        agent,
		Capabilities: capabilities,
		Description:  description,
	}
}

// RegisterGoverned 注册携治理元数据与默认上下文模式的子代理（Phase 7）。
// gov 声明 {purpose, data_scope, tools, risk_class} 治理目录项；
// mode 为该子代理被委派时的默认上下文模式（isolated/summary 上下文防火墙）。
func (r *CollaborationRegistry) RegisterGoverned(
	id string,
	agent Agent,
	description string,
	gov *AgentGovernance,
	mode domainagent.CollaborationContextMode,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.agents[id] = &AgentEntry{
		Agent:       agent,
		Description: description,
		Governance:  gov,
		ContextMode: mode,
	}
}

// GetGovernance 取子代理治理元数据；未注册或未声明时返回 nil。
func (r *CollaborationRegistry) GetGovernance(name string) *AgentGovernance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok {
		return nil
	}
	return entry.Governance
}

// GetContextMode 取子代理默认上下文模式；未注册或未声明时按 summary 兜底。
func (r *CollaborationRegistry) GetContextMode(name string) domainagent.CollaborationContextMode {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok || entry.ContextMode == "" {
		return domainagent.ContextModeSummary
	}
	return entry.ContextMode
}

// FindAgents finds agents that match the required skills.
func (r *CollaborationRegistry) FindAgents(requiredSkills []string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	matches := make([]string, 0)
	for id, entry := range r.agents {
		if r.matchesSkills(entry, requiredSkills) {
			matches = append(matches, id)
		}
	}
	return matches
}

// Get retrieves an agent by ID.
func (r *CollaborationRegistry) Get(id string) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[id]
	if !ok {
		return nil, false
	}
	return entry.Agent, true
}

// matchesSkills checks if an agent has the required skills.
func (r *CollaborationRegistry) matchesSkills(entry *AgentEntry, requiredSkills []string) bool {
	if len(requiredSkills) == 0 {
		return true
	}

	agentSkills := make(map[string]bool)
	for _, cap := range entry.Capabilities {
		agentSkills[cap.Name] = true
	}

	for _, skill := range requiredSkills {
		if !agentSkills[skill] {
			return false
		}
	}
	return true
}

// List returns all registered agent IDs.
func (r *CollaborationRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	return ids
}

// GetByName retrieves an agent by name with error return.
// This is the preferred method for tool-based collaboration.
func (r *CollaborationRegistry) GetByName(name string) (Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	return entry.Agent, nil
}

// GetDescription retrieves the description of an agent.
func (r *CollaborationRegistry) GetDescription(name string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok {
		return "", fmt.Errorf("agent not found: %s", name)
	}
	return entry.Description, nil
}

// GetCapabilities retrieves the capabilities of an agent.
func (r *CollaborationRegistry) GetCapabilities(name string) ([]AgentCapability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	return entry.Capabilities, nil
}

// ListWithDescriptions returns all agents with their descriptions.
func (r *CollaborationRegistry) ListWithDescriptions() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]AgentInfo, 0, len(r.agents))
	for name, entry := range r.agents {
		infos = append(infos, AgentInfo{
			Name:         name,
			Description:  entry.Description,
			Capabilities: entry.Capabilities,
		})
	}
	return infos
}
