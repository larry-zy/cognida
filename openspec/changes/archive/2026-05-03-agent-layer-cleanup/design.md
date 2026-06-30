## Context

当前 Agent 模块采用 Clean Architecture 分层，但存在多处违反依赖倒置原则的情况：

1. **Infrastructure → Application 依赖**：`adapter/agent/rag.go` 导入了 `application/usecases/rag` 和 `application/usecases/agent/tools`
2. **职责混淆**：编排逻辑（`AgentOrchestrator`）在 Domain 层定义为接口，但实际是应用层职责
3. **类型重复**：`ChatChunk`, `AgentRequest/Response` 在 Domain 和 Application 层都有定义
4. **实现错位**：`BaseAgent` 具体实现放在 Application 层

项目使用 Cloudwego Eino 框架作为 AI 能力基础设施，需要与其良好集成。

## Goals / Non-Goals

**Goals:**
- 修正依赖方向，确保 Infrastructure → Domain ← Application
- 统一层级职责：Domain 定义接口，Application 编排用例，Infrastructure 提供实现
- 消除重复的类型定义
- 保持与 Eino 框架的兼容性

**Non-Goals:**
- 不改变对外 API（HTTP/gRPC 接口保持不变）
- 不改变 Agent 的业务行为
- 不重构其他模块（rag, kb 等）

## Decisions

### D1: Domain 层只定义核心实体和仓储接口

**决策**：Domain 层保留纯领域概念，移除运行时 DTO。

**理由**：
- Domain 层应该独立于外部框架和协议
- `ChatChunk`, `AgentRequest/Response` 是应用层数据传输对象，不应在 Domain 层

**变更**：
```go
// domain/agent/entity.go - 只保留
type Agent struct { ID, Name, Type, Status, Config ... }
type AgentConfig struct { MaxIterations, EnableSmartRetrieval ... }
type Tool struct { ID, Name, Type, Enabled ... }
type AgentStatus / AgentType // 值对象

// 移除
type ChatChunk // → application/dto.go
type ToolCallRecord // → application/dto.go
type AgentRequest / AgentResponse // → application/dto.go
```

### D2: 编排是应用层职责，不在 Domain 定义

**决策**：移除 Domain 层的 `AgentOrchestrator` 接口。

**理由**：
- 编排是应用层用例的职责（如何协调多个 Agent 执行）
- Domain 层应关注单个 Agent 的业务规则

**变更**：
```go
// 移除 domain/repository.go 中的
type AgentOrchestrator interface { ... }

// 在 application 层定义编排用例
type AgentExecutor interface {
    Execute(ctx context.Context, req *AgentRequest) (*AgentResponse, error)
}
```

### D3: 工具注册表作为领域服务

**决策**：将 `ToolRegistry` 重新定位为领域服务接口。

**理由**：
- 工具是 Agent 领域的核心概念
- 注册表管理工具的生命周期，属于领域逻辑

**变更**：
```go
// domain/agent/service.go
type ToolRegistry interface {
    Register(tool Tool) error
    Get(name string) (Tool, bool)
    List() []Tool
}

type ToolExecutor interface {
    Execute(ctx context.Context, name string, input string) (string, error)
}
```

### D4: BaseAgent 移至 Infrastructure 层

**决策**：将 `BaseAgent` 移至 `infrastructure/agent/base_agent.go`。

**理由**：
- `BaseAgent` 是具体实现，依赖 Eino 框架
- Infrastructure 层负责外部依赖集成

**变更**：
```go
// infrastructure/agent/base_agent.go
type BaseAgent struct {
    model    model.ToolCallingChatModel
    tools    []tool.BaseTool
    // 实现 domain.Agent 接口（如果定义了）
}
```

### D5: Adapter 依赖 Domain 层接口

**决策**：修正 `adapter/agent/` 的依赖方向。

**理由**：
- Adapter 是 Infrastructure 层，不应依赖 Application 层
- 应依赖 Domain 层定义的服务接口

**变更**：
```go
// 修改前
import "link/internal/application/usecases/agent/tools"

// 修改后
import "link/internal/domain/agent"
```

### D6: Hooks 接口在 Domain 定义，实现在 Infrastructure

**决策**：在 Domain 层定义 Hook 接口，Infrastructure 提供具体实现。

**理由**：
- Hook 是 Agent 领域的核心扩展机制
- 具体实现依赖 LLM 等外部服务

**变更**：
```go
// domain/agent/service.go
type HookService interface {
    Before(ctx context.Context, message string) (context.Context, string, error)
    After(ctx context.Context, resp interface{}) error
}

// infrastructure/agent/hooks/conclusion.go
type ConclusionGenerator struct { ... } // 实现 HookService
```

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| 破坏现有代码兼容性 | 分阶段迁移，保留旧接口标记为 deprecated |
| Eino 框架集成问题 | `BaseAgent` 保留 Eino 相关类型，不引入不必要的抽象 |
| 测试覆盖不足 | 重构前补充测试，重构后验证测试通过 |
| 循环依赖风险 | 严格遵循依赖规则，使用 go tool 检查 |

## Migration Plan

### 阶段 1: Domain 层重构（不破坏兼容性）
1. 创建新的 `domain/agent/service.go` 定义服务接口
2. 在 `entity.go` 中标记过时类型为 deprecated
3. 添加转换函数

### 阶段 2: Application 层调整
1. 移除 `eino_agent.go` 中重复的接口定义
2. 移动 `BaseAgent` 到 Infrastructure 层
3. 更新 `dto.go` 添加缺失的转换函数

### 阶段 3: Infrastructure 层修正
1. 修正 `adapter/agent/` 的导入
2. 实现 `agent/base_agent.go`
3. 更新 `hooks/` 实现 Domain 接口

### 阶段 4: 清理
1. 移除 deprecated 代码
2. 更新所有测试
3. 验证依赖方向（`go mod graph` | grep infrastructure）

### Rollback 策略
- 每个 PR 独立，可单独回滚
- 使用 Git 分支保护，确保 CI 通过
- 保留旧接口直到所有调用方迁移完成

## Open Questions

1. **Q**: 是否需要定义 Domain 层的 `Agent` 接口？
   - **A**: 当前 Eino 框架有自己的 Agent 概念。建议：
     - Domain 层定义业务行为接口（如 `Execute`, `GetStatus`）
     - Infrastructure 层适配 Eino 框架

2. **Q**: `ToolRegistry` 是放在 Domain 还是 Application？
   - **A**: 放在 Domain 作为领域服务，Application 层通过依赖注入使用

3. **Q**: 编排模式的接口定义在哪里？
   - **A**: 在 Application 层定义编排用例接口（如 `SequentialExecutor`, `ParallelExecutor`）
