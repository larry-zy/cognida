// Package tools 提供全局工具注册表
package tools

import (
	"fmt"
	"sort"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	agentframework "link/internal/service/agent/framework"
)

// GlobalRegistry 全局工具注册表单例
var GlobalRegistry = NewGlobalRegistry()

// GlobalToolRegistry 全局工具注册表
// 支持工具分组管理和统一访问
type GlobalToolRegistry struct {
	core   *agentframework.ToolRegistryImpl // 核心 ToolRegistry
	groups map[string][]string               // 分组 -> 工具名列表
	mu     sync.RWMutex
}

// NewGlobalRegistry 创建全局工具注册表
func NewGlobalRegistry() *GlobalToolRegistry {
	return &GlobalToolRegistry{
		core:   agentframework.NewToolRegistry(),
		groups: make(map[string][]string),
	}
}

// Register 注册工具到指定分组
// 如果分组为空，则注册到默认分组
func (r *GlobalToolRegistry) Register(group string, t tool.BaseTool) error {
	if t == nil {
		return fmt.Errorf("tool cannot be nil")
	}

	// 获取工具名称
	info, err := t.Info(nil)
	if err != nil {
		return fmt.Errorf("failed to get tool info: %w", err)
	}

	// 注册到核心注册表
	if err := r.core.RegisterDirect(info.Name, t); err != nil {
		return fmt.Errorf("failed to register tool %s: %w", info.Name, err)
	}

	// 添加到分组
	r.mu.Lock()
	defer r.mu.Unlock()

	if group == "" {
		group = "default"
	}

	r.groups[group] = append(r.groups[group], info.Name)

	return nil
}

// RegisterBatch 批量注册工具到指定分组
func (r *GlobalToolRegistry) RegisterBatch(group string, tools []tool.BaseTool) error {
	for _, t := range tools {
		if err := r.Register(group, t); err != nil {
			return err
		}
	}
	return nil
}

// Get 获取工具
func (r *GlobalToolRegistry) Get(name string) (tool.BaseTool, bool) {
	return r.core.GetDirect(name)
}

// GetByGroup 获取分组下所有工具
func (r *GlobalToolRegistry) GetByGroup(group string) []tool.BaseTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names, exists := r.groups[group]
	if !exists {
		return []tool.BaseTool{}
	}

	tools := make([]tool.BaseTool, 0, len(names))
	for _, name := range names {
		if t, ok := r.core.GetDirect(name); ok {
			tools = append(tools, t)
		}
	}

	return tools
}

// ListGroups 列出所有分组
func (r *GlobalToolRegistry) ListGroups() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	groups := make([]string, 0, len(r.groups))
	for group := range r.groups {
		groups = append(groups, group)
	}

	sort.Strings(groups)
	return groups
}

// ListTools 列出所有工具名
func (r *GlobalToolRegistry) ListTools() []string {
	return r.core.List()
}

// ListToolsByGroup 列出指定分组的工具名
func (r *GlobalToolRegistry) ListToolsByGroup(group string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names, exists := r.groups[group]
	if !exists {
		return []string{}
	}

	// 返回副本
	result := make([]string, len(names))
	copy(result, names)
	return result
}

// GetToolInfo 获取工具信息
func (r *GlobalToolRegistry) GetToolInfo(name string) (schema.ToolInfo, bool) {
	return r.core.GetInfo(name)
}

// ListToolInfo 列出所有工具信息
func (r *GlobalToolRegistry) ListToolInfo() []schema.ToolInfo {
	return r.core.ListInfo()
}

// Size 返回工具总数
func (r *GlobalToolRegistry) Size() int {
	return r.core.Size()
}

// Unregister 注销工具
func (r *GlobalToolRegistry) Unregister(name string) error {
	if err := r.core.Unregister(name); err != nil {
		return err
	}

	// 从分组中移除
	r.mu.Lock()
	defer r.mu.Unlock()

	for group, names := range r.groups {
		for i, n := range names {
			if n == name {
				r.groups[group] = append(names[:i], names[i+1:]...)
				break
			}
		}
	}

	return nil
}

// ========================================
// 便捷访问函数
// ========================================

// GetTool 获取工具（全局便捷函数）
func GetTool(name string) (tool.BaseTool, bool) {
	return GlobalRegistry.Get(name)
}

// GetToolsByGroup 获取分组工具（全局便捷函数）
func GetToolsByGroup(group string) []tool.BaseTool {
	return GlobalRegistry.GetByGroup(group)
}

// ListAllTools 列出所有工具名（全局便捷函数）
func ListAllTools() []string {
	return GlobalRegistry.ListTools()
}

// ListAllGroups 列出所有分组（全局便捷函数）
func ListAllGroups() []string {
	return GlobalRegistry.ListGroups()
}

// GetToolInfo 获取工具信息（全局便捷函数）
func GetToolInfo(name string) (schema.ToolInfo, bool) {
	return GlobalRegistry.GetToolInfo(name)
}

// RegistrySize 返回工具总数（全局便捷函数）
func RegistrySize() int {
	return GlobalRegistry.Size()
}
