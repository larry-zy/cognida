# Agent Collaboration Tools

LLM 可调用的多 Agent 协作工具，实现 Agent 间灵活的协作模式。

## 概述

协作工具允许 LLM 自主决策何时、如何与其他 Agent 协作。与框架驱动的协作（如 Sequential、Parallel）不同，这些工具让 Agent 能够在对话中动态决定协作方式。

## 协作模式对比

| 层级 | 模式 | 控制方 | 适用场景 |
|------|------|--------|---------|
| **L3** | 工具协作 | LLM | 灵活场景、自主决策 |
| **L2** | 框架协作 | 框架 | 复杂任务、自动分解 |
| **L1** | 流程编排 | 代码 | 固定流程、确定性执行 |

### L1 - 流程编排 (Orchestration)

固定流程的 Agent 编排，代码驱动。

```go
// 顺序执行
orch := orchestration.NewSequential(
    agent1,
    agent2,
    agent3,
)

// 条件路由
orch := orchestration.NewConditional().
    When(condition1, agent1).
    When(condition2, agent2).
    Default(agent3)
```

### L2 - 框架协作 (Framework Collaboration)

自动任务分解、分发和聚合。

```go
decomposer := collaboration.NewTaskDecomposer(llm)
dispatcher := collaboration.NewTaskDispatcher(registry, 3)
aggregator := collaboration.NewResultAggregator()

plan := decomposer.Decompose(ctx, query)
result := dispatcher.Dispatch(ctx, plan)
response := aggregator.Aggregate(ctx, result, query)
```

### L3 - 工具协作 (Tool Collaboration) ⭐ 新增

LLM 自主决策协作方式。

```go
registry := agent.NewCollaborationRegistry()
registry.Register("expert", expertAgent, capabilities, "Domain expert")

agent := agent.New(llm).
    Name("Coordinator").
    WithCollaboration(registry, agent.EnableAllCollaboration()).
    Build()
```

## 协作工具

### 1. DelegateTool - 委派工具

将任务委派给指定 Agent，等待结果返回。

**使用场景：** 需要专业 Agent 处理特定子任务

**参数：**
- `agent_name` (必填): 目标 Agent 名称
- `task` (必填): 任务描述

**特性：**
- 循环检测：防止 A → B → A 的循环委派
- 超时控制：默认 30 秒超时
- 错误处理：返回可用 Agent 列表

### 2. AskTool - 咨询工具

向其他 Agent 咨询问题，保留控制权。

**使用场景：** 需要额外信息或验证意见

**参数：**
- `agent_name` (必填): 目标 Agent 名称
- `question` (必填): 咨询问题

**特性：**
- 控制权保留：咨询后原 Agent 继续处理
- 快速响应：默认 30 秒超时

### 3. HandoffTool - 转移工具

转移对话控制权给目标 Agent。

**使用场景：** 任务类型变更，需要转交专家

**参数：**
- `agent_name` (必填): 目标 Agent 名称
- `context` (可选): 传递的上下文信息

**特性：**
- 控制权转移：后续对话由目标 Agent 处理
- 上下文传递：携带当前对话信息
- 长超时：默认 60 秒，支持复杂任务

## 使用示例

### 基础用法

```go
package main

import (
    "context"
    "log"

    "github.com/cloudwego/eino/components/model"
    "link/internal/infrastructure/agent"
)

func main() {
    llm := // ... 初始化 LLM

    // 创建 Agent Registry
    registry := agent.NewCollaborationRegistry()

    // 注册专业 Agent
    researchAgent := // ... 创建研究 Agent
    registry.Register(
        "researcher",
        researchAgent,
        []agent.AgentCapability{
            {Name: "web_search", Description: "Search web for information"},
        },
        "Web research expert",
    )

    codeAgent := // ... 创建代码 Agent
    registry.Register(
        "coder",
        codeAgent,
        []agent.AgentCapability{
            {Name: "code_generation", Description: "Generate code"},
        },
        "Code generation expert",
    )

    // 创建启用了协作的协调 Agent
    coordinator := agent.New(llm).
        Name("Coordinator").
        Prompt("You are a task coordinator. Delegate tasks to specialized agents when needed.").
        WithCollaboration(registry, agent.EnableAllCollaboration()).
        Build()

    // 使用 Agent
    resp, err := coordinator.Chat(ctx, "Research latest AI trends and write a summary")
    if err != nil {
        log.Fatal(err)
    }
    log.Println(resp.Content)
    // LLM 可能会：
    // 1. 调用 delegate_to_agent 委派给 researcher
    // 2. 调用 ask_agent 咨询 coder 关于技术细节
    // 3. 综合结果返回
}
```

### 选择性启用

```go
// 只启用委派和咨询，不启用转移
coordinator := agent.New(llm).
    Name("Coordinator").
    WithCollaboration(registry,
        agent.EnableDelegate(),
        agent.EnableAsk(),
    ).
    Build()
```

### 上下文传递

```go
// LLM 可能调用：
// handoff_to(agent_name="expert", context="User has been asking about pricing...")
// 这会将上下文传递给 expert Agent
```

## 错误处理

### Agent 不存在

```json
{
  "error": "agent 'expert' not found. Available agents:\n  - researcher: Web research expert\n  - coder: Code generation expert"
}
```

### 循环检测

```json
{
  "error": "collaboration loop detected: agent_a -> agent_b -> agent_a"
}
```

### 错误检查

```go
err := // ... 调用 Agent

if agent.IsCollabLoopError(err) {
    // 处理循环检测错误
    loopErr := err.(*agent.CollabLoopError)
    log.Printf("Loop detected: %v", loopErr.Path)
}

if agent.IsAgentNotFound(err) {
    // 处理 Agent 不存在错误
    log.Printf("Agent not found")
}
```

## API 参考

### Builder 方法

```go
// WithCollaboration 启用协作工具
func (b *Builder) WithCollaboration(
    registry *AgentRegistry,
    opts ...CollaborationOption,
) *Builder

// 选项函数
func EnableDelegate() CollaborationOption
func EnableAsk() CollaborationOption
func EnableHandoff() CollaborationOption
func EnableAllCollaboration() CollaborationOption
```

### AgentRegistry 方法

```go
// 注册 Agent
func (r *AgentRegistry) Register(
    name string,
    agent Agent,
    capabilities []AgentCapability,
    description string,
)

// 按名称获取 Agent
func (r *AgentRegistry) GetByName(name string) (Agent, error)

// 获取 Agent 描述
func (r *AgentRegistry) GetDescription(name string) (string, error)

// 获取 Agent 能力
func (r *AgentRegistry) GetCapabilities(name string) ([]AgentCapability, error)

// 列出所有 Agent 及描述
func (r *AgentRegistry) ListWithDescriptions() []AgentInfo
```

## 最佳实践

1. **明确 Agent 能力描述**：帮助 LLM 理解何时使用哪个 Agent

2. **合理设置超时**：根据任务复杂度调整工具超时时间

3. **监控协作链路**：使用上下文跟踪协作路径

4. **错误恢复**：LLM 应根据错误信息调整策略，而非盲目重试

5. **混合使用**：结合 L1/L2/L3 协作模式，根据场景选择
