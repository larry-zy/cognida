// Package agentstate 提供会话态领域门面 AgentState（架构加固 Phase 6）。
//
// 背景：会话态原本散在 memory / convcontext / resultstore / pendingaction /
// uibinding / semanticcache 六处各自为政，"谁在什么时候读写哪块态"缺乏单点可推理。
//
// 设计（薄门面，design.md §6 决策 C）：AgentState 是按 owner(tenant:session)
// 聚合的门面——
//   - 统一生命周期词汇：New/Load → 经具名子域 mutate → Persist/Expire；
//   - 统一访问入口：Results/Pending/UI/Cache 具名子域访问器；
//   - 不吞子域实现：各 store 保留自己的后端与 TTL/过期语义
//     （resultstore 30min / pendingaction 10min 原子消费 / uibinding 24h /
//      semanticcache 24h + 版本失效），门面只委派、不上提统一 TTL。
//
// 读/写路径边界（防写路径污染读投影，任务 6.4）：
//   - 写路径 = chat.session（UI 会话生命周期，internal/service/chat）+ 各 store 的 Put；
//   - 读路径 = conversation.memory（跨轮记忆只读投影，convcontext.Build 回放 messages）。
//
// UI 会话写入不得直接改写跨轮记忆的只读投影；二者仅经本门面的具名入口交汇。
//
// 明确排除（非 per-session 会话态，不纳入门面，Phase 6 决策 2026-07-05）：
//   - framework/memory_registry：agent 定义元信息注册中心，与会话态正交；
//   - context/{window,layers,budget}：无状态纯函数计算，无生命周期可收敛。
package agentstate

import (
	"context"

	"cognida/internal/service/agent/pendingaction"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/agent/semanticcache"
	"cognida/internal/service/agent/uibinding"
)

// Owner 是会话态的归属标识（租户 + 会话），全部子域按此隔离读写。
type Owner struct {
	TenantID  int64
	SessionID string
}

// Key 返回子域后端使用的归属键（tenant:session），与 resultstore.OwnerKey 同口径。
func (o Owner) Key() string {
	return resultstore.OwnerKey(o.TenantID, o.SessionID)
}

// SessionMemory 是跨轮记忆读写编排在门面下的最小生命周期入口（conversation.memory
// 读路径 + 写穿透）。由 memory.MemoryService 结构化满足；可为 nil（该门面实例未接线记忆）。
// 只暴露会话级过期所需的 ClearSession，避免把 memory 的重依赖拽入门面。
type SessionMemory interface {
	ClearSession(ctx context.Context, sessionID string) error
}

// Stores 聚合按 owner 归属的会话态子域后端，由组合根一次性装配后传入门面。
// 门面不构造子域，只持有其读写入口——保留各子域内聚实现与各自 TTL。
type Stores struct {
	// Results 数据结果（data-by-reference），30min TTL，可重复读。
	Results resultstore.Store
	// Pending 待确认危险操作，10min TTL，原子 Consume（一次性 + token 防伪）。
	Pending pendingaction.Store
	// UI UI surface 回调绑定，24h 会话 TTL，可重复读。
	UI uibinding.Store
	// Cache 受信查询缓存，24h TTL + 语义模型版本失效。
	Cache semanticcache.Cache
	// Memory 跨轮记忆读写编排（可选）；nil 表示该门面实例未接线记忆，
	// 过期语义退化为仅 TTL 型子域自然回收。
	Memory SessionMemory
}

// AgentState 是按 owner 聚合的会话态门面。零值不可用，须经 New/Load 构造。
type AgentState struct {
	owner  Owner
	stores Stores
}

// New 建立一个会话态门面句柄（新会话或已知 owner 的即时装配）。
func New(owner Owner, stores Stores) *AgentState {
	return &AgentState{owner: owner, stores: stores}
}

// Load 建立既有会话的门面句柄。薄门面下各子域后端为 TTL 惰性存储，Load 仅绑定
// owner 并返回句柄；后端不可用不算失败（子域各自降级），故当前恒不返回错误。
// 保留 (ctx, error) 签名以便后续需要预热/校验时无破坏性扩展。
func Load(ctx context.Context, owner Owner, stores Stores) (*AgentState, error) {
	return &AgentState{owner: owner, stores: stores}, nil
}

// Owner 返回归属标识。
func (a *AgentState) Owner() Owner { return a.owner }

// OwnerKey 返回子域后端使用的归属键（tenant:session）。
func (a *AgentState) OwnerKey() string { return a.owner.Key() }

// Results 返回数据结果子域读写入口（可能为 nil，调用方 nil-guard 降级）。
func (a *AgentState) Results() resultstore.Store { return a.stores.Results }

// Pending 返回待确认操作子域读写入口（可能为 nil）。
func (a *AgentState) Pending() pendingaction.Store { return a.stores.Pending }

// UI 返回 UI 绑定子域读写入口（可能为 nil）。
func (a *AgentState) UI() uibinding.Store { return a.stores.UI }

// Cache 返回受信查询缓存子域读写入口（可能为 nil）。
func (a *AgentState) Cache() semanticcache.Cache { return a.stores.Cache }

// Persist 会话态写路径的单一收口点。薄门面下各子域在各自 Put/写穿透时即落库
// （resultstore/uibinding/semanticcache 落 Redis、pendingaction 暂存、memory
// 写穿透 MySQL），故 Persist 当前为语义锚点：标记"会话态持久化在此收口"，无需
// 额外批量刷写。保留方法以便后续把跨子域事务性刷写收敛于此。
func (a *AgentState) Persist(ctx context.Context) error { return nil }

// Expire 会话级过期收口。TTL 型子域（result/ui/cache/pending）由后端 TTL 自然
// 回收，无需显式删除；跨轮记忆无 TTL，若接线则经门面显式清理，防止会话结束后
// 记忆无界增长。
func (a *AgentState) Expire(ctx context.Context) error {
	if a.stores.Memory != nil {
		return a.stores.Memory.ClearSession(ctx, a.owner.SessionID)
	}
	return nil
}
