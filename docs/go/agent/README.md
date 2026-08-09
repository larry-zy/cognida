# Agent Framework

CognidaGo Agent框架是基于[Cloudwego Eino](https://github.com/cloudwego/eino)构建的统一AI Agent开发框架，提供简洁的API和强大的编排能力。

## 特性

- 🎯 **统一接口** - 简单的Chat/Stream API
- 🔧 **Builder模式** - 流畅的Agent构建体验
- 🛠️ **工具集成** - 无缝集成工具注册表
- 🧠 **记忆管理** - 内置会话历史支持
- 🔄 **编排模式** - Sequential、Parallel、Loop、Supervisor等
- 🔌 **中间件** - 可插拔的请求/响应处理
- 📊 **可观测性** - OpenTelemetry追踪 + Prometheus指标

## 快速开始

```go
import (
    "link/internal/application/usecases/agent"
    "link/internal/infrastructure/llm/chat"
)

// 创建Agent
myAgent, _ := agent.New(nil).
    Name("Assistant").
    Prompt("You are a helpful assistant.").
    WithToolModel(model).
    Build(ctx)

// 聊天
response, _ := myAgent.Chat(ctx, "Hello!")
fmt.Println(response.Content)
```

## 核心 API

### Agent 接口

```go
type Agent interface {
    // 处理消息并返回完整响应
    Chat(ctx context.Context, message string) (*Response, error)

    // 流式处理消息
    Stream(ctx context.Context, message string) (<-chan *Chunk, error)

    // 返回Agent名称
    Name() string
}
```

### Builder 选项

| 方法 | 描述 |
|------|------|
| `Name(name)` | 设置Agent名称 |
| `Prompt(prompt)` | 设置系统提示 |
| `WithToolModel(model)` | 设置支持工具调用的模型 |
| `Tools(tools...)` | 手动指定工具 |
| `ToolsAutoSelect()` | LLM自动选择工具 |
| `WithRegistry(registry)` | 设置工具注册表 |
| `Before(hook)` | 添加前置钩子 |
| `After(hook)` | 添加后置钩子 |
| `Middleware(mw...)` | 添加中间件 |
| `WithMaxIterations(n)` | 设置最大工具迭代次数 |

## 工具集成

### 手动指定工具

```go
myAgent, _ := agent.New(nil).
    Name("Researcher").
    Tools(ragTool, searchTool).
    WithToolModel(model).
    Build(ctx)
```

### 自动工具选择

```go
myAgent, _ := agent.New(nil).
    Name("AutoAgent").
    ToolsAutoSelect().
    WithRegistry(tools.GetDefaultRegistry()).
    WithToolModel(model).
    Build(ctx)
```

## 编排模式

### Sequential - 串行执行

```go
agent := orchestlation.Sequential(
    researchAgent,
    writeAgent,
    reviewAgent,
)
// 依次执行: Research → Write → Review
```

### Parallel - 并行执行

```go
agent := orchestlation.Parallel(
    optimistAgent,
    pessimistAgent,
)
// 同时执行，聚合结果
```

### Conditional - 条件路由

```go
agent := orchestlation.Branch(
    func(msg string) bool { return len(msg) < 50 },
    quickAgent,      // 短消息
    detailedAgent,   // 长消息
)
```

### Loop - 循环执行

```go
agent := orchestration.Loop(
    improverAgent,
    func(resp *Response) bool {
        return len(resp.Content) < 500  // 继续直到足够长
    },
)
```

### Supervisor - 主管模式

```go
agent := orchestration.Supervisor(
    coordinatorAgent,
    worker1, worker2, worker3,
)
// coordinator决定路由到哪个worker
```

## 中间件

### 内置中间件

```go
// 日志中间件
logging := agent.NewLoggingMiddleware(os.Stdout, "[AGENT]")

// 指标中间件
metrics := agent.NewMetricsMiddleware()

// 链式组合
agent.New(nil).
    Middleware(logging, metrics).
    Build(ctx)
```

### 自定义中间件

```go
type MyMiddleware struct{}

func (m *MyMiddleware) Before(ctx context.Context, message string) (context.Context, string, error) {
    // 请求前处理
    return ctx, message, nil
}

func (m *MyMiddleware) After(ctx context.Context, resp *Response) error {
    // 响应后处理
    return nil
}
```

## 可观测性

### OpenTelemetry 追踪

```go
import "link/internal/infrastructure/telemetry"

// 包装Agent以启用追踪
agent := telemetry.WrapAgent(myAgent)
```

### Prometheus 指标

可用指标：
- `agent.requests.total` - 请求总数
- `agent.latency` - 请求延迟
- `agent.tool_calls.total` - 工具调用次数
- `agent.errors.total` - 错误总数
- `agent.requests.active` - 活跃请求数

## 迁移指南

### 从原生 Eino 迁移

**之前 (Eino):**
```go
resp, err := model.Generate(ctx, messages)
```

**之后 (Agent框架):**
```go
agent, _ := agent.New(model).Build(ctx)
resp, err := agent.Chat(ctx, "user message")
```

### 主要优势

1. **更简洁** - 不需要手动管理messages数组
2. **工具调用** - 内置工具循环，无需手动处理
3. **流式响应** - 统一的流式API
4. **编排能力** - 开箱即用的Agent组合模式

## 更多示例

- [基础用法](../../examples/agent/basic/)
- [工具使用](../../examples/agent/tools/)
- [编排模式](../../examples/agent/orchestration/)
- [中间件](../../examples/agent/middleware/)
