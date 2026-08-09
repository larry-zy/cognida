package framework

import (
	"context"
	"strings"

	"cognida/internal/model/agent"
)

// AgentBuildFunc 是预设/初始化器提供的实例构建工厂（thunk）。
//
// 之所以用闭包而非在 AgentSpec 里直接引用 *tools.ToolRegistry：`tools` 包 import
// 了 `framework`，framework 若反向 import tools 会形成环。故实例装配（按名取工具、
// 绑定模型、挂 Hook）留在 initializer/preset 层（可自由 import tools），Build 闭包
// 捕获所需依赖，framework 只持有这个函数类型。
type AgentBuildFunc func(ctx context.Context) (Agent, error)

// AgentSpec 声明式 Agent 描述：元信息（供 UI 列表/详情）+ 构建工厂（产出可运行实例）。
//
// 简单 Agent 的 Build 直接用共享装配逻辑；复杂预设（委派/记忆/PER 管道）在 Build 里
// 保留其 bespoke 装配——「混合式」在声明式描述之外给复杂预设留逃生口。
type AgentSpec struct {
	ID          string
	Name        string
	Description string
	Type        agent.AgentType
	// ToolNames 声明该 Agent 绑定的工具名（仅作元信息展示；实际装配在 Build 内完成）。
	ToolNames []string
	// Aliases 兼容历史 agentID（如 agent-text2sql-001）：实例在这些别名下同样可取，
	// 但元信息只登记主 ID，避免 UI 出现重复条目。
	Aliases []string
	// Metadata 额外元信息（version/pattern/phases 等）；未含 "tools" 键时由 ToolNames 补齐。
	Metadata map[string]string
	// Build 构建可运行实例；为 nil 时 RegisterSpec 报错。
	Build AgentBuildFunc
}

// definition 从 spec 生成注册用元信息（合并 ToolNames 到 metadata["tools"]）。
func (s AgentSpec) definition() *agent.AgentDefinition {
	meta := make(map[string]string, len(s.Metadata)+1)
	for k, v := range s.Metadata {
		meta[k] = v
	}
	if _, ok := meta["tools"]; !ok {
		meta["tools"] = strings.Join(s.ToolNames, ",")
	}
	return &agent.AgentDefinition{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Type:        s.Type,
		Status:      agent.AgentStatusIdle,
		Metadata:    meta,
	}
}

// AgentInstanceRegistry 在元信息注册表（agent.AgentRegistry）之上，额外按 AgentSpec
// 构建并持有可运行 Agent 实例，供 orchestrator 路由。
type AgentInstanceRegistry interface {
	agent.AgentRegistry
	// RegisterSpec 按 spec 构建实例并登记（元信息 + 可运行实例）。
	RegisterSpec(ctx context.Context, spec AgentSpec) error
	// GetInstance 取可运行实例（含别名）。
	GetInstance(agentID string) (Agent, bool)
}
