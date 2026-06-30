// Package agent provides Agent infrastructure implementations.
package framework

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"link/internal/model/agent"
)

// ========================================
// ToolRegistry - Domain ToolRegistry 接口实现
// ========================================

// toolRegistryImpl implements the domain.ToolRegistry interface.
// It manages Eino framework tools and adapts them to domain concepts.
type ToolRegistryImpl struct {
	mu     sync.RWMutex
	tools  map[string]tool.BaseTool
	enabled map[string]bool
}

// Ensure ToolRegistryImpl implements domain.ToolRegistry
var _ ToolRegistry = (*ToolRegistryImpl)(nil)

// NewToolRegistry creates a new tool registry implementing domain.ToolRegistry.
func NewToolRegistry() *ToolRegistryImpl {
	return &ToolRegistryImpl{
		tools:   make(map[string]tool.BaseTool),
		enabled: make(map[string]bool),
	}
}

// Register registers a tool. It accepts an Eino BaseTool and wraps it as a domain Tool.
func (r *ToolRegistryImpl) Register(domainTool agent.Tool) error {
	if domainTool.ID == "" {
		return fmt.Errorf("tool ID cannot be empty")
	}

	// TODO: Convert domain.Tool to Eino tool.BaseTool
	// For now, store the domain tool metadata
	r.mu.Lock()
	defer r.mu.Unlock()

	// Store the tool - in future this will hold the actual Eino tool
	r.tools[strings.ToLower(domainTool.ID)] = nil // Placeholder
	r.enabled[strings.ToLower(domainTool.ID)] = domainTool.Enabled

	return nil
}

// RegisterEinoTool registers an Eino tool directly (convenience method).
func (r *ToolRegistryImpl) RegisterEinoTool(name string, einoTool tool.BaseTool) error {
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if einoTool == nil {
		return fmt.Errorf("tool cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[strings.ToLower(name)] = einoTool
	r.enabled[strings.ToLower(name)] = true

	return nil
}

// Unregister removes a tool by name.
func (r *ToolRegistryImpl) Unregister(toolName string) error {
	name := strings.ToLower(toolName)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return fmt.Errorf("tool not found: %s", toolName)
	}

	delete(r.tools, name)
	delete(r.enabled, name)

	return nil
}

// Get retrieves a tool by name.
// Returns a domain.Tool representation.
func (r *ToolRegistryImpl) Get(toolName string) (agent.Tool, bool) {
	name := strings.ToLower(toolName)

	r.mu.RLock()
	defer r.mu.RUnlock()

	einoTool, exists := r.tools[name]
	if !exists {
		return agent.Tool{}, false
	}

	// Convert Eino tool to domain tool
	return r.einoToDomainTool(name, einoTool), true
}

// GetEinoTool retrieves an Eino tool by name (internal use).
func (r *ToolRegistryImpl) GetEinoTool(toolName string) (tool.BaseTool, bool) {
	name := strings.ToLower(toolName)

	r.mu.RLock()
	defer r.mu.RUnlock()

	t, exists := r.tools[name]
	return t, exists
}

// GetTools returns all enabled tools as Eino tools.
func (r *ToolRegistryImpl) GetTools() []tool.BaseTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]tool.BaseTool, 0, len(r.tools))
	for name, einoTool := range r.tools {
		if r.enabled[name] && einoTool != nil {
			tools = append(tools, einoTool)
		}
	}
	return tools
}

// GetToolsByNames retrieves tools by their names.
func (r *ToolRegistryImpl) GetToolsByNames(names []string) ([]tool.BaseTool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]tool.BaseTool, 0, len(names))
	for _, name := range names {
		lowerName := strings.ToLower(name)
		if t, exists := r.tools[lowerName]; exists && r.enabled[lowerName] && t != nil {
			tools = append(tools, t)
		}
	}
	return tools, nil
}

// List returns all registered tool names.
func (r *ToolRegistryImpl) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// ListDomain returns all registered tools as domain Tools.
func (r *ToolRegistryImpl) ListDomain() []agent.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	domainTools := make([]agent.Tool, 0, len(r.tools))
	for name, einoTool := range r.tools {
		domainTools = append(domainTools, r.einoToDomainTool(name, einoTool))
	}

	return domainTools
}

// ListByType returns tools filtered by type.
func (r *ToolRegistryImpl) ListByType(toolType agent.ToolType) []agent.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []agent.Tool
	for name, einoTool := range r.tools {
		domainTool := r.einoToDomainTool(name, einoTool)
		if domainTool.Type == toolType {
			result = append(result, domainTool)
		}
	}

	return result
}

// Enable marks a tool as enabled.
func (r *ToolRegistryImpl) Enable(toolName string) error {
	name := strings.ToLower(toolName)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return fmt.Errorf("tool not found: %s", toolName)
	}

	r.enabled[name] = true
	return nil
}

// Disable marks a tool as disabled.
func (r *ToolRegistryImpl) Disable(toolName string) error {
	name := strings.ToLower(toolName)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return fmt.Errorf("tool not found: %s", toolName)
	}

	r.enabled[name] = false
	return nil
}

// IsEnabled checks if a tool is enabled.
func (r *ToolRegistryImpl) IsEnabled(toolName string) bool {
	name := strings.ToLower(toolName)

	r.mu.RLock()
	defer r.mu.RUnlock()

	enabled, exists := r.enabled[name]
	return exists && enabled
}

// einoToDomainTool converts an Eino tool to a domain Tool.
func (r *ToolRegistryImpl) einoToDomainTool(name string, einoTool tool.BaseTool) agent.Tool {
	domainTool := agent.Tool{
		ID:      name,
		Name:    name,
		Enabled: r.enabled[name],
	}

	if einoTool != nil {
		// Try to get info from Eino tool
		info, err := einoTool.Info(context.Background())
		if err == nil {
			domainTool.Description = info.Desc
			// Map Eino tool types to domain types
			domainTool.Type = r.mapToolType(info.Desc)
		}
	}

	return domainTool
}

// mapToolType maps tool description to domain ToolType.
func (r *ToolRegistryImpl) mapToolType(desc string) agent.ToolType {
	descLower := strings.ToLower(desc)

	if strings.Contains(descLower, "search") || strings.Contains(descLower, "搜索") {
		return agent.ToolTypeSearch
	}
	if strings.Contains(descLower, "retriev") || strings.Contains(descLower, "检索") || strings.Contains(descLower, "rag") {
		return agent.ToolTypeRetrieval
	}
	if strings.Contains(descLower, "agent") {
		return agent.ToolTypeAgent
	}

	return agent.ToolTypeUtility
}

// ========================================
// Global Registry (Singleton)
// ========================================

var (
	globalRegistry ToolRegistry
	registryOnce   sync.Once
)

// GetGlobalRegistry returns the global tool registry.
func GetGlobalRegistry() ToolRegistry {
	registryOnce.Do(func() {
		globalRegistry = NewToolRegistry()
	})
	return globalRegistry
}

// ========================================
// Core-compatible methods (for migration from core package)
// ========================================

// RegisterDirect registers a tool directly (core-compatible method).
func (r *ToolRegistryImpl) RegisterDirect(name string, t tool.BaseTool) error {
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if t == nil {
		return fmt.Errorf("tool cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[strings.ToLower(name)] = t
	r.enabled[strings.ToLower(name)] = true

	return nil
}

// GetDirect retrieves a tool.BaseTool directly (core-compatible method).
func (r *ToolRegistryImpl) GetDirect(name string) (tool.BaseTool, bool) {
	lowerName := strings.ToLower(name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	t, exists := r.tools[lowerName]
	return t, exists
}

// GetInfo retrieves tool info by name (core-compatible method).
func (r *ToolRegistryImpl) GetInfo(name string) (schema.ToolInfo, bool) {
	lowerName := strings.ToLower(name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	t, exists := r.tools[lowerName]
	if !exists {
		return schema.ToolInfo{}, false
	}

	info, err := t.Info(context.Background())
	if err != nil {
		return schema.ToolInfo{}, false
	}

	return *info, true
}

// ListInfo returns info for all registered tools (core-compatible method).
func (r *ToolRegistryImpl) ListInfo() []schema.ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]schema.ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		if t != nil {
			if info, err := t.Info(context.Background()); err == nil {
				infos = append(infos, *info)
			}
		}
	}
	return infos
}

// Size returns the number of registered tools (core-compatible method).
func (r *ToolRegistryImpl) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tools)
}
