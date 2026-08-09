package framework

import (
	"context"
	"fmt"
	"sync"

	"cognida/internal/model/agent"
)

// SpecRegistry 声明式 Agent 注册表：按 AgentSpec 构建并持有可运行实例，
// 同时经嵌入的 *MemoryRegistry 维护元信息（实现 agent.AgentRegistry，供 handler
// 列表/详情）。取代了此前散落各包的 `var xxxAgentInstance` 全局单例 + GetAgentByID switch。
//
// 编译期断言其满足 AgentInstanceRegistry。
type SpecRegistry struct {
	*MemoryRegistry
	// mu 仅保护 instances；元信息的并发由内嵌 MemoryRegistry 自有锁保护。
	mu        sync.RWMutex
	instances map[string]Agent
	// wrap 在实例入库前对其做统一装饰（如遥测/追踪）。为空则不装饰。
	// 由组合根注入（telemetry.WrapAgent），避免 framework→telemetry 反向依赖：
	// framework 只持有 func(Agent)Agent 钩子，装饰实现留在上层包。
	wrap func(Agent) Agent
}

var _ AgentInstanceRegistry = (*SpecRegistry)(nil)

// NewSpecRegistry 创建声明式注册表。
func NewSpecRegistry() *SpecRegistry {
	return &SpecRegistry{
		MemoryRegistry: NewMemoryRegistry(),
		instances:      make(map[string]Agent),
	}
}

// SetWrap 注入实例装饰钩子（组合根在注册任何 spec 前调用一次）。
// 传入后，RegisterSpec 构建出的每个 Agent 实例都会先经 wrap 包装再入库，
// 从而对「全部 agent」统一叠加遥测/追踪等横切能力。传 nil 关闭装饰。
func (r *SpecRegistry) SetWrap(wrap func(Agent) Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wrap = wrap
}

// RegisterSpec 按 spec 构建实例并登记：先构建（失败即返回，不留半注册态），
// 再写元信息，最后按主 ID + 别名存实例。
func (r *SpecRegistry) RegisterSpec(ctx context.Context, spec AgentSpec) error {
	if spec.ID == "" {
		return fmt.Errorf("agent spec ID 不能为空")
	}
	if spec.Build == nil {
		return fmt.Errorf("agent spec %s 缺少 Build 工厂", spec.ID)
	}

	inst, err := spec.Build(ctx)
	if err != nil {
		return fmt.Errorf("构建 agent %s 失败: %w", spec.ID, err)
	}
	if inst == nil {
		return fmt.Errorf("agent %s 的 Build 返回了 nil 实例", spec.ID)
	}

	if err := r.MemoryRegistry.Register(ctx, spec.definition()); err != nil {
		return err
	}

	r.mu.Lock()
	// 统一装饰：主 ID 与所有别名共享同一被装饰实例（一次包装，别名复用）。
	if r.wrap != nil {
		if wrapped := r.wrap(inst); wrapped != nil {
			inst = wrapped
		}
	}
	r.instances[spec.ID] = inst
	for _, alias := range spec.Aliases {
		if alias != "" && alias != spec.ID {
			r.instances[alias] = inst
		}
	}
	r.mu.Unlock()
	return nil
}

// GetInstance 取可运行 Agent 实例（供 orchestrator 路由）。
// agentID 为空时回退到 "default"，与旧 GetAgentByID/orchestrator 行为一致。
func (r *SpecRegistry) GetInstance(agentID string) (Agent, bool) {
	if agentID == "" {
		agentID = "default"
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.instances[agentID]
	return a, ok
}

// 确保 SpecRegistry 满足 agent.AgentRegistry（经内嵌 MemoryRegistry 提升的方法集）。
var _ agent.AgentRegistry = (*SpecRegistry)(nil)
