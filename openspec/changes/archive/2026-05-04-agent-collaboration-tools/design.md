## Context

**当前状态**：
- Link 有 `orchestration/` 包提供 Sequential、Parallel、Supervisor、Conditional、Loop、Func 等编排模式
- Link 有 `collaboration/` 包提供：
  - `TaskDecomposer` - LLM 驱动的任务分解
  - `AgentRegistry` - Agent 注册表（已有基础实现）
  - `TaskDispatcher` - 任务分发器（支持依赖解析、并行执行）
  - `ResultAggregator` - 结果聚合器（支持多种聚合策略）
- 现有协作模式都是**框架驱动**的固定流程，LLM 无法自主决策何时、如何协作
- Eino 框架已支持 Tool 调用，Agent 可通过工具扩展能力

**现有协作能力回顾**：

```
// 现有：框架驱动的任务分解 + 分发 + 聚合
plan := decomposer.Decompose(ctx, query)
result := dispatcher.Dispatch(ctx, plan)
response := aggregator.Aggregate(ctx, result, query)

// 问题：LLM 无法自主决定是否需要协作、与谁协作
```

**目标**：提供 LLM 可调用的协作工具，实现 LLM 自主决策协作方式

**约束条件**：
- 必须基于 Eino 框架的 `tool.InvokableTool` 接口
- 向后兼容，不能破坏现有 Agent 代码和 collaboration/ 包
- 不实现 subagent 包装（用户明确排除）
- 遵循 Clean Architecture，协作逻辑放在 application 层
- 与现有 collaboration/ 包**互补共存**，不重复实现已有能力

## Goals / Non-Goals

**Goals:**
- 提供 LLM 可调用的协作工具，实现自主决策协作
- 支持 Delegate（委派）、Ask（咨询）、Handoff（转移）三种协作模式
- 扩展现有 `AgentRegistry`，添加 Tool 友好的方法
- Builder 模式扩展，支持便捷启用协作能力
- 协作循环检测和错误恢复

**Non-Goals:**
- Subagent 包装（`WrapAgentAsTool`）暂不实现
- 协作历史追踪和持久化
- 复杂的共识投票机制（已有 ResultAggregator 负责）
- 任务分解逻辑（已有 TaskDecomposer 负责）
- 结果聚合逻辑（已有 ResultAggregator 负责）

## Decisions

### 1. 协作工具架构

**决策**：协作工具实现为独立的 Eino Tool，每个工具对应一种协作模式

```go
// collaboration/tools.go
type DelegateTool struct {
    registry *AgentRegistry
    maxDepth int  // 防止无限循环
}

type AskTool struct {
    registry *AgentRegistry
}

type HandoffTool struct {
    registry *AgentRegistry
}
```

**理由**：
- 每个 Tool 可独立使用，用户按需启用
- 符合 Eino 框架的设计模式
- 便于扩展新的协作模式

**替代方案**：统一的一个 `CollaborationTool`，通过参数区分模式
- **弃用原因**：违反单一职责原则，LLM 难以理解不同模式语义

### 2. AgentRegistry 扩展策略

**决策**：扩展现有 `AgentRegistry`，添加 Tool 友好的方法

**现有方法**（`collaboration/task.go`）：
```go
func (r *AgentRegistry) Register(id string, agent agent.Agent, capabilities []AgentCapability, description string)
func (r *AgentRegistry) FindAgents(requiredSkills []string) []string
func (r *AgentRegistry) Get(id string) (agent.Agent, bool)
func (r *AgentRegistry) List() []string
```

**需要新增**：
```go
// 按名称查找，返回 error 而非 bool（更适合 Tool 场景）
func (r *AgentRegistry) GetByName(name string) (agent.Agent, error)

// 获取 Agent 元数据，用于 Tool Info 描述生成
func (r *AgentRegistry) GetDescription(name string) (string, error)
func (r *AgentRegistry) GetCapabilities(name string) ([]AgentCapability, error)

// 列出所有 Agent 及其描述，用于错误提示
func (r *AgentRegistry) ListWithDescriptions() []AgentInfo
```

**理由**：
- 复用现有实现，减少重复代码
- 新方法更适配 Tool 调用场景
- 向后兼容，不修改现有方法签名

### 3. Builder 扩展方式

**决策**：添加 `WithCollaboration()` 方法，接受 CollaborationOption

```go
func (b *Builder) WithCollaboration(registry *collaboration.AgentRegistry, opts ...CollaborationOption) *Builder
```

**理由**：
- 符合现有 Builder 风格
- 选项模式支持灵活配置
- 不影响现有 Builder 链式调用

### 4. 与现有协作能力的关系

**决策**：三种协作模式共存，各自适用不同场景

```
┌─────────────────────────────────────────────────────────────┐
│                    协作模式分层                              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  [L3 - LLM 驱动协作] ← 本方案新增                            │
│  ├── DelegateTool - LLM 自主委派任务                        │
│  ├── AskTool - LLM 自主咨询其他 Agent                       │
│  └── HandoffTool - LLM 自主转移控制权                       │
│                                                             │
│  [L2 - 框架驱动协作] ← 现有 collaboration/ 包                │
│  ├── TaskDecomposer - 自动任务分解                         │
│  ├── TaskDispatcher - 依赖解析 + 并行分发                  │
│  └── ResultAggregator - 多种聚合策略                       │
│                                                             │
│  [L1 - 流程编排] ← 现有 orchestration/ 包                   │
│  ├── Sequential - 顺序执行                                  │
│  ├── Parallel - 并行执行                                    │
│  ├── Supervisor - 路由分发                                  │
│  ├── Conditional - 条件分支                                 │
│  └── Loop/Func - 循环/自定义                                │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**选择原则**：
- **L1 编排**：固定流程、不需要 LLM 决策
- **L2 协作**：复杂任务、需要自动分解和并行执行
- **L3 工具**：灵活场景、需要 LLM 自主决策

### 5. 协作工具语义设计

| 工具 | 语义 | 适用场景 | 与现有能力对比 |
|------|------|---------|---------------|
| `DelegateTool` | 将任务委派给指定 Agent，等待结果返回 | 需要专业 Agent 处理特定子任务 | 比 TaskDispatcher 更灵活，LLM 决定何时委派 |
| `AskTool` | 向其他 Agent 咨询，获取信息补充，保留控制权 | 需要额外信息或验证意见 | 现有能力无对应，全新场景 |
| `HandoffTool` | 转移对话控制权，后续由目标 Agent 处理 | 任务类型变更，需要转交专家 | 类似 Conditional 路由，但由 LLM 决定 |

### 6. 循环检测机制

**决策**：在 DelegateTool 中实现基于上下文的链路跟踪

```go
type contextKey struct{}

// 委派链路存储在 context 中
func withDelegatePath(ctx context.Context, agentName string) context.Context {
    path := getDelegatePath(ctx)
    path = append(path, agentName)
    return context.WithValue(ctx, contextKey{}, path)
}

func detectLoop(ctx context.Context, targetAgent string) bool {
    path := getDelegatePath(ctx)
    for _, agent := range path {
        if agent == targetAgent {
            return true  // 检测到循环
        }
    }
    return false
}
```

**理由**：
- 利用 Eino 的 context 传递机制
- 无需额外状态存储
- 自动支持并发安全

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| 协作循环（A 委派给 B，B 委派回 A） | 在 Tool 中检测并拒绝已委派过的 Agent |
| Agent Registry 线程安全 | 现有实现已使用 sync.RWMutex 保护 |
| 协作失败导致主流程中断 | Tool 返回错误信息给 LLM，由 LLM 决定后续策略 |
| LLM 滥用协作工具 | 通过 System Prompt 明确工具使用场景，设置合理的描述 |
| 与现有 collaboration 包混淆 | 文档清晰说明三种模式的区别和使用场景 |

## Migration Plan

**阶段 1：扩展现有 AgentRegistry**
1. 添加 `GetByName()`、`GetDescription()` 等新方法
2. 添加 `AgentInfo` 结构体用于返回完整信息
3. 保持现有方法向后兼容

**阶段 2：实现协作工具**
1. 创建 `collaboration/tools.go`
2. 实现 `DelegateTool`（含循环检测）
3. 实现 `AskTool`
4. 实现 `HandoffTool`

**阶段 3：Builder 集成**
1. 扩展 `eino_builder.go` 添加 `WithCollaboration()`
2. 添加协作选项定义和配置结构
3. 实现工具自动注入逻辑

**阶段 4：错误处理和测试**
1. 定义协作相关错误常量和类型
2. 编写单元测试
3. 编写集成测试
4. 更新文档

**回滚策略**：所有改动都是新增或扩展，不修改现有代码，可直接删除新增代码回滚

## Open Questions

1. ~~Agent 能力描述格式~~
   - **已解决**：现有 `AgentCapability` 结构体已支持

2. **协作超时处理**：
   - 当前倾向：使用 context 超时，由调用方控制
   - 后续可能：在 Tool 层添加默认超时配置

3. **多租户隔离**：
   - **倾向**：不需要，Agent 已在应用层处理租户隔离
   - 待确认：是否有跨租户协作需求

4. **Handoff 上下文传递格式**：
   - 需要明确：传递哪些上下文信息（历史消息、元数据等）
   - 待设计：上下文压缩策略，避免超出 Token 限制
