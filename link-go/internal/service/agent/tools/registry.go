// Package tools 提供工具注册表
package tools

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"link/internal/service/agent/agentstate"
	agentframework "link/internal/service/agent/framework"
	"link/internal/service/agent/pendingaction"
	"link/internal/service/agent/resultstore"
	"link/internal/service/agent/uibinding"
)

// ToolRegistry 工具注册表：支持工具分组管理和统一访问。
// 依赖经 ToolDeps 显式注入，构造时按分组注册全部工具，替代旧的包级全局单例与 init() 副作用。
type ToolRegistry struct {
	core   *agentframework.ToolRegistryImpl // 核心 ToolRegistry
	groups map[string][]string              // 分组 -> 工具名列表
	mu     sync.RWMutex
	deps   ToolDeps // 注入的工具依赖
	ops    *opTools // 操作工具依赖载体（registerOperationTools 装配；handler confirm-resume 复用）
}

// NewToolRegistry 构造工具注册表（唯一公开构造入口）：
// 校验必需依赖 → 归一化可选依赖 → 按分组注册全部工具（顺序与原 init 流程一致）。
func NewToolRegistry(deps ToolDeps) (*ToolRegistry, error) {
	if deps.SQLDB == nil {
		return nil, fmt.Errorf("构造 ToolRegistry 缺少必需依赖 SQLDB")
	}
	if deps.GetSchemaDB == nil {
		deps.GetSchemaDB = deps.SQLDB
	}

	r := &ToolRegistry{
		core:   agentframework.NewToolRegistry(),
		groups: make(map[string][]string),
		deps:   deps,
	}

	// 注册顺序与原 InitializeTools 逐一对应，保证分组/工具名/装配副作用一致。
	registrars := []func() error{
		r.registerRAGTools,
		r.registerSQLTools,
		r.registerWebTools,
		r.registerKBTools,
		r.registerDataTools,
		r.registerSemanticTools,
		r.registerGraphTools,
		r.registerAnalyticsTools,
		r.registerRenderTools,
		r.registerOperationTools,
		r.registerSkillTools,
	}
	for _, register := range registrars {
		if err := register(); err != nil {
			return nil, err
		}
	}

	return r, nil
}

// ResultStore 取当前 Result Store（供 handler 层做 UI 回调按引用取数）。
func (r *ToolRegistry) ResultStore() resultstore.Store {
	return r.deps.ResultStore
}

// PendingStore 取当前待确认操作存储（供 handler 层 confirm-resume 消费）。
func (r *ToolRegistry) PendingStore() pendingaction.Store {
	return r.deps.Operation.Pending
}

// UIBinding 取当前 UI 绑定存储（供 handler 层回调路由取 surface 绑定）。
func (r *ToolRegistry) UIBinding() uibinding.Store {
	return r.deps.UIBinding
}

// SessionState 按 (tenant, session) 装配会话态门面（薄门面 AgentState）：把散在
// 各子域存储的读写经统一 owner 归属聚合，供 handler 层按会话读写会话态。
// 子域后端沿用注册表持有的实例（可为 nil，门面调用方各自 nil-guard 降级）。
//
// 注：此处刻意不接线 Stores.Memory（ToolDeps 不持有记忆服务，且当前无 Expire 活调用方）。
// 经本工厂取得的门面 Expire 因而只做 TTL 型子域自然回收、不清跨轮记忆；将来若需在
// 会话结束时经门面清记忆，应先把记忆服务纳入 ToolDeps 再在此填入 Memory 字段。
func (r *ToolRegistry) SessionState(tenantID int64, sessionID string) *agentstate.AgentState {
	return agentstate.New(
		agentstate.Owner{TenantID: tenantID, SessionID: sessionID},
		agentstate.Stores{
			Results: r.deps.ResultStore,
			Pending: r.deps.Operation.Pending,
			UI:      r.deps.UIBinding,
			Cache:   r.deps.SemanticCache,
		},
	)
}

// ExecuteConfirmedMutation 执行已人工确认的写操作（供 handler 层 confirm-resume 调用）。
func (r *ToolRegistry) ExecuteConfirmedMutation(ctx context.Context, action *pendingaction.PendingAction) (*SQLMutateResult, error) {
	if r.ops == nil {
		return nil, fmt.Errorf("操作工具未初始化")
	}
	return r.ops.ExecuteConfirmedMutation(ctx, action)
}

// RecordUnsupportedConfirmKind 对不支持的确认类型留审计痕（供 handler 层调用，未初始化时静默跳过）。
func (r *ToolRegistry) RecordUnsupportedConfirmKind(ctx context.Context, action *pendingaction.PendingAction) {
	if r.ops == nil {
		return
	}
	r.ops.RecordUnsupportedConfirmKind(ctx, action)
}

// FetchSchema 用注册表持有的库连接与数据源路由查询表结构（供 HTTP handler 复用）。
func (r *ToolRegistry) FetchSchema(ctx context.Context, databaseID, tableName string) (*GetSchemaResult, error) {
	db := r.deps.GetSchemaDB
	if db == nil {
		db = r.deps.SQLDB
	}
	return FetchSchema(ctx, db, r.deps.DatasourceProvider, databaseID, tableName)
}

// Register 注册工具到指定分组
// 如果分组为空，则注册到默认分组
func (r *ToolRegistry) Register(group string, t tool.BaseTool) error {
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
func (r *ToolRegistry) RegisterBatch(group string, tools []tool.BaseTool) error {
	for _, t := range tools {
		if err := r.Register(group, t); err != nil {
			return err
		}
	}
	return nil
}

// Get 获取工具
func (r *ToolRegistry) Get(name string) (tool.BaseTool, bool) {
	return r.core.GetDirect(name)
}

// GetByGroup 获取分组下所有工具
func (r *ToolRegistry) GetByGroup(group string) []tool.BaseTool {
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
func (r *ToolRegistry) ListGroups() []string {
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
func (r *ToolRegistry) ListTools() []string {
	return r.core.List()
}

// ListToolsByGroup 列出指定分组的工具名
func (r *ToolRegistry) ListToolsByGroup(group string) []string {
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
func (r *ToolRegistry) GetToolInfo(name string) (schema.ToolInfo, bool) {
	return r.core.GetInfo(name)
}

// ListToolInfo 列出所有工具信息
func (r *ToolRegistry) ListToolInfo() []schema.ToolInfo {
	return r.core.ListInfo()
}

// Size 返回工具总数
func (r *ToolRegistry) Size() int {
	return r.core.Size()
}

// Unregister 注销工具
func (r *ToolRegistry) Unregister(name string) error {
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
