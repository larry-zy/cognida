# Agent Framework - Code-First Design

## Why

当前项目虽然使用了 eino 框架，但存在以下问题：

1. **直接使用 eino 底层 API** - 代码中大量使用 `adk.NewChatModelAgent` 等底层 API，使用繁琐
2. **缺乏便捷的构建器** - 每次创建 Agent 都需要写大量重复代码
3. **编排模式使用复杂** - eino ADK 的 Supervisor、Sequential、Parallel 等编排方式需要深入了解才能使用
4. **缺少业务层抽象** - 没有统一的 Agent 接口，不同地方的使用方式不一致

**核心理念变化**：从"配置驱动"转向"代码驱动"

- ❌ 不是：通过 YAML 配置定义 Agent 和编排
- ✅ 而是：提供原子能力 + 便捷构建器，用代码自由组合

这样设计的好处：
1. **类型安全** - 全部是 Go 代码，编译期检查
2. **IDE 友好** - 自动补全、重构支持
3. **灵活强大** - 可以添加任意业务逻辑
4. **易于调试** - 断点、日志都很直接
5. **符合 Go 惯例** - 像 http.Handler、sql.DB 一样的接口设计

## What Changes

### 核心变更

- **新增 Agent 核心接口**：简洁的 Agent 接口（Chat、Stream）
- **新增 Agent 构建器**：链式 API，快速创建 Agent
- **新增编排构建器**：Sequential、Parallel、Supervisor 等编排模式
- **集成现有 Tool Registry**：复用已有工具，无需重新实现
- **集成现有 RAG/Session**：复用已有 RAG 和 Memory 能力
- **支持自动工具选择**：LLM 自动决定使用哪些工具
- **保持基础设施配置**：只配置 API Key、数据库连接等

### 具体变更

1. **新增 `internal/agent/` 核心包**
   - `agent/` - Agent 接口和构建器
   - `orchestration/` - 编排模式
   - `integration/` - 集成现有能力（Tool Registry、RAG、Session）

2. **复用现有能力**
   - `internal/application/usecases/agent/tools/` - Tool Registry
   - `internal/application/usecases/rag/` - RAG Service
   - `internal/application/usecases/chat/` - Session/Memory

3. **提供示例代码** - `examples/` 展示各种用法

4. **保持不变**
   - 现有业务逻辑
   - HTTP 接口（可选集成新框架）

## Capabilities

### New Capabilities

- `agent-core`: 核心 Agent 接口和构建器
- `agent-tool-integration`: 集成现有 Tool Registry
- `agent-tool-auto-select`: 支持自动工具选择（LLM Function Calling）
- `agent-rag-integration`: 集成现有 RAG Service
- `agent-memory-integration`: 集成现有 Session/Memory
- `agent-orchestration`: 编排模式（Sequential、Parallel、Supervisor、Loop、Conditional）
- `agent-middleware`: 中间件支持（日志、指标、限流等）

## Design Principles

### 1. 代码优先

```go
// ✅ 这样用 - 代码控制
agent := agent.New(model).
    Name("searcher").
    Tools(tools.WebSearch(key)).
    Build(ctx)

// ❌ 不要这样 - 配置驱动
# YAML 配置文件...
```

### 2. 简洁接口

```go
type Agent interface {
    Chat(ctx context.Context, message string) (*Response, error)
    Stream(ctx context.Context, message string) (<-chan *Chunk, error)
}
```

### 3. 链式构建

```go
agent := agent.New(model).
    Name("my-agent").
    Prompt("You are...").
    Tools(tool1, tool2).
    Before(preprocess).
    After(postprocess).
    Build(ctx)
```

### 4. 自由组合

```go
// Agent 可以任意组合
pipeline := orchestration.Sequential(agent1, agent2)
parallel := orchestration.Parallel(agent3, agent4)
supervisor := orchestration.Supervisor(coordinator, worker1, worker2)

// 编排也是 Agent
complex := orchestration.Sequential(
    pipeline,
    parallel,
    supervisor,
)
```

## Directory Structure

```
link-go/
├── internal/
│   └── agent/                           # Agent 框架核心（新增）
│       ├── agent/                       # Agent 接口和构建器
│       │   ├── agent.go                 # Agent 接口
│       │   ├── builder.go               # 构建器
│       │   └── response.go              # Response 和 Chunk
│       │
│       ├── orchestration/               # 编排模式
│       │   ├── sequential.go
│       │   ├── parallel.go
│       │   ├── supervisor.go
│       │   ├── conditional.go
│       │   ├── loop.go
│       │   └── func.go
│       │
│       └── integration/                 # 集成现有能力
│           ├── tools.go                 # 集成 Tool Registry
│           ├── rag.go                   # 集成 RAG Service
│           └── memory.go                # 集成 Session/Memory
│
├── internal/application/usecases/
│   ├── agent/tools/                     # Tool Registry（复用）
│   │   ├── registry.go                  # 已存在
│   │   ├── web_search.go                # 已存在
│   │   ├── rag_query.go                 # 已存在
│   │   └── ...
│   ├── rag/                             # RAG Service（复用）
│   └── chat/                            # Session/Memory（复用）
│
├── examples/                             # 使用示例
│   ├── basic/
│   │   └── auto_tool_select.go          # 自动工具选择示例
│   ├── data_collection/
│   └── research_assistant/
│
└── config/
    └── default.yaml                      # 基础设施配置
```

## Impact

### 代码影响

- **新增代码**：约 2000 行框架代码 + 示例
- **删除代码**：可以考虑替换现有的复杂 orchestrator
- **重构范围**：用户可以选择性地迁移到新框架

### 不影响

- 现有业务逻辑可以继续工作
- HTTP 接口保持兼容（可选升级）

## Migration Strategy

1. **Phase 1**: 实现核心框架
2. **Phase 2**: 提供示例代码
3. **Phase 3**: 用户可以选择性迁移
4. **Phase 4**: 废弃旧的复杂实现

## Examples

### 基础 Agent - 自动工具选择

```go
import "link/internal/application/usecases/agent/tools"

// 使用现有 Tool Registry
registry := tools.GetDefaultRegistry()

// 创建智能助手 - LLM 自动选择工具
assistant := agent.New(model).
    Name("assistant").
    ToolsAutoSelect().  // 自动从注册中心选择
    Build(ctx)

// LLM 自动选择 web_search
resp1, _ := assistant.Chat(ctx, "最新 AI Agent 趋势")

// LLM 自动选择 rag_query
resp2, _ := assistant.Chat(ctx, "公司产品架构")
```

### 基础 Agent - 手动指定工具

```go
agent := agent.New(model).
    Name("doc-searcher").
    Tools(
        tools.RAGQueryTool(),  // 只用 RAG
    ).
    Build(ctx)

resp, err := agent.Chat(ctx, "搜索文档")
```

### 基础 Agent - 从注册中心获取

```go
agent := agent.New(model).
    Name("assistant").
    ToolsFromRegistry("rag_query", "web_search").
    Build(ctx)
```

### 带会话历史的 Agent

```go
agent := agent.New(model).
    Name("assistant").
    ToolsAutoSelect().
    WithSession(sessionID).  // 集成现有 Session
    Build(ctx)

// 自动保持对话上下文
resp1, _ := agent.Chat(ctx, "我叫张三")
resp2, _ := agent.Chat(ctx, "我叫什么名字？")
```

### 编排

```go
pipeline := orchestration.Sequential(
    urlFinder,
    extractor,
    cleaner,
)

resp, err := pipeline.Chat(ctx, "搜集数据")
```

### 自定义逻辑

```go
customAgent := orchestration.Func(func(ctx context.Context, msg string) (*agent.Response, error) {
    // 任意业务逻辑
    resp, _ := someAgent.Chat(ctx, msg)
    // 处理...
    return processedResp, nil
})
```
