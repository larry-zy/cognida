// Package agent provides Agent infrastructure implementations.
package framework

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"cognida/internal/model/agent"
)

// ToolRegistryImpl stores executable Eino tools. Domain agent.Tool values are
// metadata only and deliberately cannot be registered as executable tools.
type ToolRegistryImpl struct {
	mu      sync.RWMutex
	tools   map[string]tool.BaseTool
	enabled map[string]bool
}

// Ensure ToolRegistryImpl implements the framework registry used by Builder.
var _ ToolRegistry = (*ToolRegistryImpl)(nil)

// NewToolRegistry creates an executable tool registry.
func NewToolRegistry() *ToolRegistryImpl {
	return &ToolRegistryImpl{
		tools:   make(map[string]tool.BaseTool),
		enabled: make(map[string]bool),
	}
}

// RegisterEinoTool registers a concrete executable, never a metadata placeholder.
func (r *ToolRegistryImpl) RegisterEinoTool(name string, einoTool tool.BaseTool) error {
	return r.RegisterDirect(name, einoTool)
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
	return t, exists && t != nil && r.enabled[name]
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
	sort.Strings(names)
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
		if err == nil && info != nil {
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
// Core-compatible methods (for migration from core package)
// ========================================

// RegisterDirect registers a tool directly (core-compatible method).
func (r *ToolRegistryImpl) RegisterDirect(name string, t tool.BaseTool) error {
	if name == "" || strings.TrimSpace(name) != name {
		return fmt.Errorf("tool name must be nonempty without surrounding whitespace")
	}
	if t == nil || isNilTool(t) {
		return fmt.Errorf("tool cannot be nil")
	}
	switch t.(type) {
	case tool.InvokableTool, tool.StreamableTool, tool.EnhancedInvokableTool, tool.EnhancedStreamableTool:
	default:
		return fmt.Errorf("tool must provide an executable invocation or stream")
	}
	info, err := t.Info(context.Background())
	if err != nil || info == nil || !strings.EqualFold(info.Name, name) {
		return fmt.Errorf("tool info must be valid and match its registered name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[strings.ToLower(name)] = t
	r.enabled[strings.ToLower(name)] = true
	return nil
}

func isNilTool(t tool.BaseTool) bool {
	value := reflect.ValueOf(t)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// GetDirect retrieves a tool.BaseTool directly (core-compatible method).
func (r *ToolRegistryImpl) GetDirect(name string) (tool.BaseTool, bool) {
	lowerName := strings.ToLower(name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	t, exists := r.tools[lowerName]
	return t, exists && t != nil && r.enabled[lowerName]
}

// GetInfo retrieves tool info by name (core-compatible method).
func (r *ToolRegistryImpl) GetInfo(name string) (schema.ToolInfo, bool) {
	lowerName := strings.ToLower(name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	t, exists := r.tools[lowerName]
	if !exists || !r.enabled[lowerName] {
		return schema.ToolInfo{}, false
	}

	info, err := t.Info(context.Background())
	if err != nil || info == nil {
		return schema.ToolInfo{}, false
	}

	return *info, true
}

// ListInfo returns info for all registered tools (core-compatible method).
func (r *ToolRegistryImpl) ListInfo() []schema.ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]schema.ToolInfo, 0, len(r.tools))
	for name, t := range r.tools {
		if t != nil && r.enabled[name] {
			if info, err := t.Info(context.Background()); err == nil && info != nil {
				infos = append(infos, *info)
			}
		}
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}

// Size returns the number of registered tools (core-compatible method).
func (r *ToolRegistryImpl) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tools)
}
