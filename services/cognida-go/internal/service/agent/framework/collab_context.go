package framework

import (
	"context"
	"errors"
)

// collabRegistryKey 是把子代理协作注册表只读注入执行上下文的私有键。
// 注入后，工具/skill handler 可经 InlineDelegate 在自身执行体内 inline 编排子代理群，
// 而无需 LLM 显式走 delegate_* 往返（上下文更干净，见 design D7/D8）。
type collabRegistryKeyType struct{}

var collabRegistryKey = collabRegistryKeyType{}

// ErrNoCollabRegistry 表示当前执行上下文未注入子代理注册表——取用方据此明确降级
// （返回「无子代理可编排」而非 panic），不影响不依赖它的工具/skill。
var ErrNoCollabRegistry = errors.New("当前执行上下文无子代理注册表，无法编排子代理")

// WithCollaborationRegistry 把协作注册表只读注入 ctx（组合根/主 run 在循环入口调用）。
// registry 为 nil 时原样返回，取用方仍走 ErrNoCollabRegistry 降级。
func WithCollaborationRegistry(ctx context.Context, registry *CollaborationRegistry) context.Context {
	if registry == nil {
		return ctx
	}
	return context.WithValue(ctx, collabRegistryKey, registry)
}

// collabRegistryFromContext 取用注入的协作注册表。
func collabRegistryFromContext(ctx context.Context) (*CollaborationRegistry, bool) {
	r, ok := ctx.Value(collabRegistryKey).(*CollaborationRegistry)
	return r, ok && r != nil
}

// HasCollaborationRegistry 供上层（skill handler/工具）判断当前是否具备 inline 编排能力。
func HasCollaborationRegistry(ctx context.Context) bool {
	_, ok := collabRegistryFromContext(ctx)
	return ok
}

// InlineDelegate 在工具/skill handler 内 inline 编排一个子代理：复用与 delegate_* 完全相同的
// 委派内核（executeDelegation）——信封契约校验、IsCyclic/MaxDepth 护栏、scope 越权护栏、
// 上下文防火墙与只回传紧凑结论。与 LLM 触发的 delegate_* 的唯一区别是「由 handler 代码直接调用」，
// 主循环只见一次 skill 调用，内部逐轮往返不回灌（design D8）。
//
// 未注入注册表时返回 ErrNoCollabRegistry，调用方据此降级为纯指导或如实报错。
func InlineDelegate(ctx context.Context, env DelegationEnvelope) (string, error) {
	registry, ok := collabRegistryFromContext(ctx)
	if !ok {
		return "", ErrNoCollabRegistry
	}
	return executeDelegation(ctx, registry, &env)
}
