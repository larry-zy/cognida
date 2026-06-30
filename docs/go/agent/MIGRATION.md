# 迁移指南：从原生 Eino 到 Agent 框架

本文档帮助您从原生 Cloudwego Eino 迁移到 LinkGo Agent 框架。

## 为什么迁移？

| 特性 | 原生 Eino | Agent 框架 |
|------|-----------|------------|
| 消息管理 | 手动维护messages数组 | 自动管理 |
| 工具调用 | 需要手动处理循环 | 内置迭代逻辑 |
| 流式响应 | 需要处理StreamReader | 统一的Channel API |
| 记忆管理 | 需要自己实现 | 内置Session支持 |
| 编排模式 | 需要自己实现 | 6种开箱即用模式 |

## 基础迁移

### 之前 (Eino)

```go
import "github.com/cloudwego/eino/components/model"

// 创建模型
model, _ := chat.NewToolCallingChatModel(ctx, config)

// 准备消息
messages := []*schema.Message{
    schema.SystemMessage("You are helpful."),
    schema.UserMessage("Hello"),
}

// 生成响应
resp, _ := model.Generate(ctx, messages)
fmt.Println(resp.Content)
```

### 之后 (Agent 框架)

```go
import "link/internal/application/usecases/agent"

// 创建Agent
agent, _ := agent.New(nil).
    Prompt("You are helpful.").
    WithToolModel(model).
    Build(ctx)

// 直接聊天
resp, _ := agent.Chat(ctx, "Hello")
fmt.Println(resp.Content)
```

## 工具调用迁移

### 之前 (Eino)

```go
// 需要手动处理工具循环
messages := []*schema.Message{...}

for i := 0; i < maxIterations; i++ {
    resp, _ := model.Generate(ctx, messages)

    if len(resp.ToolCalls) == 0 {
        break  // 没有工具调用
    }

    // 执行工具
    for _, tc := range resp.ToolCalls {
        output, _ := executeTool(tc)
        messages = append(messages,
            schema.ToolMessage(output, tc.ID),
        )
    }
}
```

### 之后 (Agent 框架)

```go
// 工具调用自动处理
agent, _ := agent.New(nil).
    Tools(myTools...).
    WithToolModel(model).
    WithMaxIterations(10).
    Build(ctx)

resp, _ := agent.Chat(ctx, "Use tools to answer")
// resp.ToolCalls 包含所有工具调用信息
```

## 流式响应迁移

### 之前 (Eino)

```go
stream, _ := model.Stream(ctx, messages)
defer stream.Close()

for {
    chunk, err := stream.Recv()
    if err != nil {
        break
    }
    fmt.Print(chunk.Content)
}
```

### 之后 (Agent 框架)

```go
ch, _ := agent.Stream(ctx, "Hello")

for chunk := range ch {
    if chunk.Done {
        break
    }
    fmt.Print(chunk.Content)
}
```

## 编排模式迁移

### 之前：手动实现

```go
// 串行执行需要手动实现
func sequential(agents []Agent, msg string) string {
    result := msg
    for _, a := range agents {
        resp, _ := a.Chat(ctx, result)
        result = resp.Content
    }
    return result
}

// 并行执行需要手动实现
func parallel(agents []Agent, msg string) []string {
    var wg sync.WaitGroup
    results := make([]string, len(agents))

    for i, a := range agents {
        wg.Add(1)
        go func(i int, a Agent) {
            defer wg.Done()
            resp, _ := a.Chat(ctx, msg)
            results[i] = resp.Content
        }(i, a)
    }
    wg.Wait()
    return results
}
```

### 之后：使用内置模式

```go
import "link/internal/application/usecases/agent/orchestration"

// 串行
seqAgent := orchestration.Sequential(agent1, agent2, agent3)
resp, _ := seqAgent.Chat(ctx, message)

// 并行
parAgent := orchestration.Parallel(agent1, agent2, agent3)
resp, _ := parAgent.Chat(ctx, message)
```

## 记忆管理迁移

### 之前：手动管理

```go
// 需要自己维护会话历史
type Session struct {
    mu       sync.Mutex
    messages []*schema.Message
}

func (s *Session) AddMessage(msg *schema.Message) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.messages = append(s.messages, msg)
}

func (s *Session) GetMessages() []*schema.Message {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.messages
}
```

### 之后：使用内置适配器

```go
import "link/internal/infrastructure/adapter/agent"

memory := agent.NewSessionMemory()

agent, _ := agent.New(nil).
    WithMemory(memory).
    WithSession("user-123").
    WithToolModel(model).
    Build(ctx)
```

## 中间件迁移

### 之前：包装函数

```go
func withLogging(agent Agent, logger *log.Logger) Agent {
    return &loggingAgent{agent, logger}
}

type loggingAgent struct {
    Agent
    logger *log.Logger
}

func (l *loggingAgent) Chat(ctx context.Context, msg string) (*Response, error) {
    l.logger.Printf("Request: %s", msg)
    resp, err := l.Agent.Chat(ctx, msg)
    l.logger.Printf("Response: %d chars", len(resp.Content))
    return resp, err
}
```

### 之后：使用中间件接口

```go
logging := &LoggingMiddleware{logger: logger}

agent, _ := agent.New(nil).
    Middleware(logging).
    WithToolModel(model).
    Build(ctx)
```

## 完整示例对比

### 之前 (Eino)

```go
func processQuery(ctx context.Context, model model.ToolCallingChatModel, query string) (string, error) {
    messages := []*schema.Message{
        schema.SystemMessage("You are a helpful assistant."),
        schema.UserMessage(query),
    }

    // 工具循环
    for i := 0; i < 5; i++ {
        resp, err := model.Generate(ctx, messages)
        if err != nil {
            return "", err
        }

        if len(resp.ToolCalls) == 0 {
            return resp.Content, nil
        }

        // 执行工具
        for _, tc := range resp.ToolCalls {
            output, _ := executeTool(tc)
            messages = append(messages,
                schema.AssistantMessage("", tc),
                schema.ToolMessage(output, tc.ID),
            )
        }
    }

    return "", errors.New("max iterations exceeded")
}
```

### 之后 (Agent 框架)

```go
func processQuery(ctx context.Context, model model.ToolCallingChatModel, query string) (string, error) {
    agent, err := agent.New(nil).
        Prompt("You are a helpful assistant.").
        Tools(getAllTools()...).
        WithToolModel(model).
        WithMaxIterations(5).
        Build(ctx)
    if err != nil {
        return "", err
    }

    resp, err := agent.Chat(ctx, query)
    if err != nil {
        return "", err
    }

    return resp.Content, nil
}
```

## 常见问题

### Q: 如何自定义工具调用逻辑？

A: 使用 `Before`/`After` 钩子或自定义中间件。

### Q: 如何实现流式工具调用？

A: 当前版本流式模式下工具调用会被自动处理。如需自定义，使用非流式API。

### Q: 如何迁移现有的工具？

A: 如果工具实现了 `tool.InvokableTool` 或 `tool.StreamableTool` 接口，可以直接使用。

### Q: 性能如何？

A: Agent框架是Eino的薄封装，性能开销极小。主要开销在中间件链执行。

### Q: 支持哪些LLM？

A: 支持所有Eino兼容的模型：OpenAI、Claude、通义千问、文心一言等。
