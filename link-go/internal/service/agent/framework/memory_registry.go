// Package agent 提供 Agent 注册中心的内存实现
package framework

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"link/internal/model/agent"
)

// MemoryRegistry 内存实现的 Agent 注册中心
type MemoryRegistry struct {
	mu     sync.RWMutex
	agents map[string]*agent.AgentDefinition

	// name 索引
	byName map[string]string
	// type 索引
	byType map[agent.AgentType][]string
}

// NewMemoryRegistry 创建内存注册中心
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{
		agents: make(map[string]*agent.AgentDefinition),
		byName: make(map[string]string),
		byType: make(map[agent.AgentType][]string),
	}
}

// Register 注册一个 Agent
func (r *MemoryRegistry) Register(ctx context.Context, def *agent.AgentDefinition) error {
	if def == nil {
		return fmt.Errorf("agent definition cannot be nil")
	}
	if def.ID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}
	if def.Name == "" {
		return fmt.Errorf("agent name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查是否已存在
	if _, exists := r.agents[def.ID]; exists {
		return fmt.Errorf("agent already registered: %s", def.ID)
	}

	// 检查名称冲突
	if existingID, nameExists := r.byName[def.Name]; nameExists {
		return fmt.Errorf("agent name already exists: %s (used by %s)", def.Name, existingID)
	}

	// 设置状态
	if def.Status == "" {
		def.Status = agent.AgentStatusIdle
	}

	// 设置时间戳
	now := currentTimeMillis()
	def.RegisteredAt = now
	def.UpdatedAt = now

	// 存储
	r.agents[def.ID] = def
	r.byName[def.Name] = def.ID
	r.byType[def.Type] = append(r.byType[def.Type], def.ID)

	log.Printf("[AgentRegistry] ✓ Registered agent: %s (ID: %s, Type: %s)", def.Name, def.ID, def.Type)
	return nil
}

// Unregister 注销一个 Agent
func (r *MemoryRegistry) Unregister(ctx context.Context, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	def, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	// 删除索引
	delete(r.byName, def.Name)

	// 从类型索引中删除
	typeList := r.byType[def.Type]
	for i, id := range typeList {
		if id == agentID {
			r.byType[def.Type] = append(typeList[:i], typeList[i+1:]...)
			break
		}
	}

	// 删除存储
	delete(r.agents, agentID)

	log.Printf("[AgentRegistry] ✗ Unregistered agent: %s (ID: %s)", def.Name, agentID)
	return nil
}

// Get 获取指定的 Agent
func (r *MemoryRegistry) Get(ctx context.Context, agentID string) (*agent.AgentDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, exists := r.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	return def, nil
}

// List 列出所有已注册的 Agent
func (r *MemoryRegistry) List(ctx context.Context) ([]*agent.AgentDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*agent.AgentDefinition, 0, len(r.agents))
	for _, def := range r.agents {
		result = append(result, def)
	}

	return result, nil
}

// FindByType 根据类型查找 Agent
func (r *MemoryRegistry) FindByType(ctx context.Context, agentType agent.AgentType) ([]*agent.AgentDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids, exists := r.byType[agentType]
	if !exists {
		return []*agent.AgentDefinition{}, nil
	}

	result := make([]*agent.AgentDefinition, 0, len(ids))
	for _, id := range ids {
		if def, ok := r.agents[id]; ok {
			result = append(result, def)
		}
	}

	return result, nil
}

// FindByName 根据名称查找 Agent
func (r *MemoryRegistry) FindByName(ctx context.Context, name string) (*agent.AgentDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, exists := r.byName[name]
	if !exists {
		return nil, fmt.Errorf("agent not found: %s", name)
	}

	return r.agents[id], nil
}

// Exists 检查 Agent 是否存在
func (r *MemoryRegistry) Exists(ctx context.Context, agentID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.agents[agentID]
	return exists, nil
}

// UpdateStatus 更新 Agent 状态
func (r *MemoryRegistry) UpdateStatus(ctx context.Context, agentID string, status agent.AgentStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	def, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	def.Status = status
	def.UpdatedAt = currentTimeMillis()

	log.Printf("[AgentRegistry] ↻ Agent %s status updated: %s", agentID, status)
	return nil
}

// Search 搜索 Agent（支持关键词）
func (r *MemoryRegistry) Search(ctx context.Context, keyword string) ([]*agent.AgentDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if keyword == "" {
		return r.List(ctx)
	}

	keyword = strings.ToLower(keyword)
	result := make([]*agent.AgentDefinition, 0)

	for _, def := range r.agents {
		if strings.Contains(strings.ToLower(def.Name), keyword) ||
			strings.Contains(strings.ToLower(def.Description), keyword) ||
			strings.Contains(strings.ToLower(def.ID), keyword) {
			result = append(result, def)
		}
	}

	return result, nil
}

// currentTimeMillis 获取当前时间戳（毫秒）
func currentTimeMillis() int64 {
	return time.Now().UnixMilli()
}
